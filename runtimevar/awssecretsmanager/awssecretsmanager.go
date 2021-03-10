// Copyright 2020 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package awssecretsmanager provides a runtimevar implementation with variables
// read from AWS Secrets Manager (https://aws.amazon.com/secrets-manager)
// Use OpenVariable to construct a *runtimevar.Variable.
//
// URLs
//
// For runtimevar.OpenVariable, awssecretsmanager registers for the scheme "awssecretsmanager".
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// awssecretsmanager exposes the following types for As:
//  - Snapshot: *secretsmanager.GetSecretValueOutput, *secretsmanager.DescribeSecretOutput
//  - Error: awserr.Error
package awssecretsmanager // import "gocloud.dev/runtimevar/awssecretsmanager"

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/google/wire"
	gcaws "gocloud.dev/aws"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, new(lazySessionOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	wire.Struct(new(URLOpener), "ConfigProvider"),
)

// URLOpener opens AWS Secrets Manager URLs like "awssecretsmanager://my-secret-var-name".
// A friendly name of the secret must be specified. You can NOT specify the Amazon Resource Name (ARN).
//
// See gocloud.dev/aws/ConfigFromURLParams for supported query parameters
// that affect the default AWS session.
//
// In addition, the following URL parameters are supported:
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//       runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//       See runtimevar.DecoderByName for supported values.
type URLOpener struct {
	// ConfigProvider must be set to a non-nil value.
	ConfigProvider client.ConfigProvider

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the options to pass to New.
	Options Options
}

// lazySessionOpener obtains the AWS session from the environment on the first
// call to OpenVariableURL.
type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	o.init.Do(func() {
		sess, err := gcaws.NewDefaultSession()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			ConfigProvider: sess,
		}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, o.err)
	}
	return o.opener.OpenVariableURL(ctx, u)
}

// Scheme is the URL scheme awssecretsmanager registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "awssecretsmanager"

// OpenVariableURL opens the variable at the URL's path. See the package doc
// for more details.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(ctx, decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}

	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(q)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, err)
	}

	configProvider.Configs = append(configProvider.Configs, overrideCfg)

	return OpenVariable(configProvider, path.Join(u.Host, u.Path), decoder, &o.Options)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which AWS Secrets Manager is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the variable name in AWS Secrets Manager.
// A friendly name of the secret must be specified. You can NOT specify the Amazon Resource Name (ARN).
// Secrets Manager returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(sess client.ConfigProvider, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(sess, name, decoder, opts)), nil
}

// state implements driver.State.
type state struct {
	val        interface{}
	rawGet     *secretsmanager.GetSecretValueOutput
	rawDesc    *secretsmanager.DescribeSecretOutput
	updateTime time.Time
	versionID  string
	err        error
}

// Value implements driver.State.Value.
func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

// UpdateTime implements driver.State.UpdateTime.
func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// As implements driver.State.As.
func (s *state) As(i interface{}) bool {
	if s.rawGet == nil {
		return false
	}
	switch p := i.(type) {
	case **secretsmanager.GetSecretValueOutput:
		*p = s.rawGet
	case **secretsmanager.DescribeSecretOutput:
		*p = s.rawDesc
	default:
		return false
	}
	return true
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	// Map aws.RequestCanceled to the more standard context package errors.
	if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == request.CanceledErrorCode {
		msg := err.Error()
		if strings.Contains(msg, "context deadline exceeded") {
			err = context.DeadlineExceeded
		} else {
			err = context.Canceled
		}
	}
	s := &state{err: err}
	if prevS == nil {
		return s
	}
	prev := prevS.(*state)
	if prev.err == nil {
		// New error.
		return s
	}
	if equivalentError(err, prev.err) {
		// Same error, return nil to indicate no change.
		return nil
	}
	return s
}

// equivalentError returns true iff err1 and err2 represent an equivalent error;
// i.e., we don't want to return it to the user as a different error.
func equivalentError(err1, err2 error) bool {
	if err1 == err2 || err1.Error() == err2.Error() {
		return true
	}
	var code1, code2 string
	if awsErr, ok := err1.(awserr.Error); ok {
		code1 = awsErr.Code()
	}
	if awsErr, ok := err2.(awserr.Error); ok {
		code2 = awsErr.Code()
	}
	return code1 != "" && code1 == code2
}

type watcher struct {
	// sess is the AWS session to use to talk to AWS.
	sess client.ConfigProvider
	// name is an ID of a secret to retrieve.
	name string
	// wait is the amount of time to wait between querying AWS.
	wait time.Duration
	// decoder is the decoder that unmarshalls the value in the param.
	decoder *runtimevar.Decoder
}

func newWatcher(sess client.ConfigProvider, name string, decoder *runtimevar.Decoder, opts *Options) *watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		sess:    sess,
		name:    name,
		wait:    driver.WaitDuration(opts.WaitDuration),
		decoder: decoder,
	}
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	var lastVersionID string
	if prev != nil {
		lastVersionID = prev.(*state).versionID
	}

	secretID := aws.String(w.name)
	svc := secretsmanager.New(w.sess)

	// GetSecretValueWithContext to get the current value and version.
	getResp, err := svc.GetSecretValueWithContext(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: secretID,
	})
	if err != nil {
		return errorState(err, prev), w.wait
	}

	if *getResp.VersionId == lastVersionID {
		// Version hasn't changed, so no change; return nil.
		return nil, w.wait
	}

	// DescribeSecretWithContext to get the LastChanged date.
	descResp, err := svc.DescribeSecretWithContext(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: secretID,
	})
	if err != nil {
		return errorState(err, prev), w.wait
	}

	// Both SecretBinary and SecretString fields are not empty
	// which could indicate some internal Secrets Manager issues.
	// Hence, return explicit error instead of choosing one field over another.
	if len(getResp.SecretBinary) > 0 && getResp.SecretString != nil && len(*getResp.SecretString) > 0 {
		err = fmt.Errorf("invalid %q response: both SecretBinary and SecretString are not empty", w.name)
		return errorState(err, prev), w.wait
	}

	data := getResp.SecretBinary

	if len(getResp.SecretBinary) == 0 {
		if getResp.SecretString == nil {
			err = fmt.Errorf("invalid %q response: both SecretBinary and SecretString are empty", w.name)
			return errorState(err, prev), w.wait
		}

		// SecretBinary is empty so use SecretString
		data = []byte(*getResp.SecretString)
	}

	// New value (or at least, new version). Decode it.
	val, err := w.decoder.Decode(ctx, data)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	return &state{
		val:        val,
		rawGet:     getResp,
		rawDesc:    descResp,
		updateTime: *descResp.LastChangedDate,
		versionID:  *getResp.VersionId,
	}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i interface{}) bool {
	switch v := err.(type) {
	case awserr.Error:
		if p, ok := i.(*awserr.Error); ok {
			*p = v
			return true
		}
	}
	return false
}

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	if awsErr, ok := err.(awserr.Error); ok {
		switch awsErr.Code() {
		case secretsmanager.ErrCodeResourceNotFoundException:
			return gcerrors.NotFound

		case secretsmanager.ErrCodeInvalidParameterException,
			secretsmanager.ErrCodeInvalidRequestException,
			secretsmanager.ErrCodeInvalidNextTokenException:
			return gcerrors.InvalidArgument

		case secretsmanager.ErrCodeEncryptionFailure,
			secretsmanager.ErrCodeDecryptionFailure,
			secretsmanager.ErrCodeInternalServiceError:
			return gcerrors.Internal

		case secretsmanager.ErrCodeResourceExistsException:
			return gcerrors.AlreadyExists

		case secretsmanager.ErrCodePreconditionNotMetException,
			secretsmanager.ErrCodeMalformedPolicyDocumentException:
			return gcerrors.FailedPrecondition

		case secretsmanager.ErrCodeLimitExceededException:
			return gcerrors.ResourceExhausted
		}
	}

	return gcerrors.Unknown
}
