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
// # URLs
//
// For runtimevar.OpenVariable, awssecretsmanager registers for the scheme "awssecretsmanager".
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// awssecretsmanager exposes the following types for As:
//   - Snapshot: (V1) *secretsmanager.GetSecretValueOutput, *secretsmanager.DescribeSecretOutput, (V2) *secretsmanagerv2.GetSecretValueOutput, *secretsmanagerv2.DescribeSecretOutput
//   - Error: (V1) awserr.Error, (V2) any error type returned by the service, notably smithy.APIError
package awssecretsmanager // import "gocloud.dev/runtimevar/awssecretsmanager"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	secretsmanagerv2 "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/smithy-go"
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
// Use "awssdk=v1" to force using AWS SDK v1, "awssdk=v2" to force using AWS SDK v2,
// or anything else to accept the default.
//
// For V1, see gocloud.dev/aws/ConfigFromURLParams for supported query parameters
// for overriding the aws.Session from the URL.
// For V2, see gocloud.dev/aws/V2ConfigFromURLParams.
//
// In addition, the following URL parameters are supported:
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//     runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//     See runtimevar.DecoderByName for supported values.
//   - wait: The poll interval, in time.ParseDuration formats.
//     Defaults to 30s.
type URLOpener struct {
	// UseV2 indicates whether the AWS SDK V2 should be used.
	UseV2 bool

	// ConfigProvider must be set to a non-nil value if UseV2 is false.
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
	if gcaws.UseV2(u.Query()) {
		opener := &URLOpener{UseV2: true}
		return opener.OpenVariableURL(ctx, u)
	}
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
	opts := o.Options
	if s := q.Get("wait"); s != "" {
		q.Del("wait")
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("open variable %v: invalid wait %q: %v", u, s, err)
		}
		opts.WaitDuration = d
	}
	if o.UseV2 {
		cfg, err := gcaws.V2ConfigFromURLParams(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("open variable %v: %v", u, err)
		}
		return OpenVariableV2(secretsmanagerv2.NewFromConfig(cfg), path.Join(u.Host, u.Path), decoder, &opts)
	}
	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(q)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, err)
	}

	configProvider.Configs = append(configProvider.Configs, overrideCfg)

	return OpenVariable(configProvider, path.Join(u.Host, u.Path), decoder, &opts)
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
	return runtimevar.New(newWatcher(false, sess, nil, name, decoder, opts)), nil
}

// OpenVariableV2 constructs a *runtimevar.Variable backed by the variable name in AWS Secrets Manager,
// using AWS SDK V2.
// A friendly name of the secret must be specified. You can NOT specify the Amazon Resource Name (ARN).
// Secrets Manager returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariableV2(client *secretsmanagerv2.Client, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(true, nil, client, name, decoder, opts)), nil
}

// state implements driver.State.
type state struct {
	val        interface{}
	rawGetV1   *secretsmanager.GetSecretValueOutput
	rawGetV2   *secretsmanagerv2.GetSecretValueOutput
	rawDescV1  *secretsmanager.DescribeSecretOutput
	rawDescV2  *secretsmanagerv2.DescribeSecretOutput
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
	switch p := i.(type) {
	case **secretsmanager.GetSecretValueOutput:
		*p = s.rawGetV1
	case **secretsmanagerv2.GetSecretValueOutput:
		*p = s.rawGetV2
	case **secretsmanager.DescribeSecretOutput:
		*p = s.rawDescV1
	case **secretsmanagerv2.DescribeSecretOutput:
		*p = s.rawDescV2
	default:
		return false
	}
	return true
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	// Map to the more standard context package error.
	if strings.Contains(err.Error(), "context deadline exceeded") {
		err = context.DeadlineExceeded
	} else if getErrorCode(err) == request.CanceledErrorCode {
		err = context.Canceled
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
	code1 := getErrorCode(err1)
	code2 := getErrorCode(err2)
	return code1 != "" && code1 == code2
}

type watcher struct {
	// useV2 indicates whether we're using clientV2.
	useV2 bool
	// sess is the AWS session to use to talk to AWS.
	sess client.ConfigProvider
	// clientV2 is the client to use when useV2 is true.
	clientV2 *secretsmanagerv2.Client
	// name is an ID of a secret to retrieve.
	name string
	// wait is the amount of time to wait between querying AWS.
	wait time.Duration
	// decoder is the decoder that unmarshalls the value in the param.
	decoder *runtimevar.Decoder
}

func newWatcher(useV2 bool, sess client.ConfigProvider, clientV2 *secretsmanagerv2.Client, name string, decoder *runtimevar.Decoder, opts *Options) *watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		useV2:    useV2,
		sess:     sess,
		clientV2: clientV2,
		name:     name,
		wait:     driver.WaitDuration(opts.WaitDuration),
		decoder:  decoder,
	}
}

func getSecretValue(ctx context.Context, svc *secretsmanager.SecretsManager, secretID string) (string, []byte, string, *secretsmanager.GetSecretValueOutput, error) {
	getResp, err := svc.GetSecretValueWithContext(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return "", nil, "", nil, err
	}
	return aws.StringValue(getResp.VersionId), getResp.SecretBinary, aws.StringValue(getResp.SecretString), getResp, nil
}

func getSecretValueV2(ctx context.Context, client *secretsmanagerv2.Client, secretID string) (string, []byte, string, *secretsmanagerv2.GetSecretValueOutput, error) {
	getResp, err := client.GetSecretValue(ctx, &secretsmanagerv2.GetSecretValueInput{
		SecretId: awsv2.String(secretID),
	})
	if err != nil {
		return "", nil, "", nil, err
	}
	return awsv2.ToString(getResp.VersionId), getResp.SecretBinary, awsv2.ToString(getResp.SecretString), getResp, nil
}

func describeSecret(ctx context.Context, svc *secretsmanager.SecretsManager, secretID string) (time.Time, *secretsmanager.DescribeSecretOutput, error) {
	descResp, err := svc.DescribeSecretWithContext(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return time.Time{}, nil, err
	}
	return aws.TimeValue(descResp.LastChangedDate), descResp, nil
}

func describeSecretV2(ctx context.Context, client *secretsmanagerv2.Client, secretID string) (time.Time, *secretsmanagerv2.DescribeSecretOutput, error) {
	descResp, err := client.DescribeSecret(ctx, &secretsmanagerv2.DescribeSecretInput{
		SecretId: awsv2.String(secretID),
	})
	if err != nil {
		return time.Time{}, nil, err
	}
	return aws.TimeValue(descResp.LastChangedDate), descResp, nil
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	var lastVersionID string
	if prev != nil {
		lastVersionID = prev.(*state).versionID
	}
	var svc *secretsmanager.SecretsManager
	if !w.useV2 {
		svc = secretsmanager.New(w.sess)
	}

	// GetParameter from S3 to get the current value and version.
	var newVersionID string
	var newValBinary []byte
	var newValString string
	var rawGetV1 *secretsmanager.GetSecretValueOutput
	var rawGetV2 *secretsmanagerv2.GetSecretValueOutput
	var err error
	if w.useV2 {
		newVersionID, newValBinary, newValString, rawGetV2, err = getSecretValueV2(ctx, w.clientV2, w.name)
	} else {
		newVersionID, newValBinary, newValString, rawGetV1, err = getSecretValue(ctx, svc, w.name)
	}
	if err != nil {
		return errorState(err, prev), w.wait
	}
	if newVersionID == lastVersionID {
		// Version hasn't changed, so no change; return nil.
		return nil, w.wait
	}
	// Both SecretBinary and SecretString fields are not empty
	// which could indicate some internal Secrets Manager issues.
	// Hence, return explicit error instead of choosing one field over another.
	if len(newValBinary) > 0 && newValString != "" {
		err = fmt.Errorf("invalid %q response: both SecretBinary and SecretString are not empty", w.name)
		return errorState(err, prev), w.wait
	}

	data := newValBinary
	if len(data) == 0 {
		if newValString == "" {
			err = fmt.Errorf("invalid %q response: both SecretBinary and SecretString are empty", w.name)
			return errorState(err, prev), w.wait
		}
		// SecretBinary is empty so use SecretString
		data = []byte(newValString)
	}

	// DescribeParameters from S3 to get the LastModified date.
	var newLastModified time.Time
	var rawDescV1 *secretsmanager.DescribeSecretOutput
	var rawDescV2 *secretsmanagerv2.DescribeSecretOutput
	if w.useV2 {
		newLastModified, rawDescV2, err = describeSecretV2(ctx, w.clientV2, w.name)
	} else {
		newLastModified, rawDescV1, err = describeSecret(ctx, svc, w.name)
	}
	if err != nil {
		return errorState(err, prev), w.wait
	}

	// New value (or at least, new version). Decode it.
	val, err := w.decoder.Decode(ctx, data)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	return &state{
		val:        val,
		rawGetV1:   rawGetV1,
		rawGetV2:   rawGetV2,
		rawDescV1:  rawDescV1,
		rawDescV2:  rawDescV2,
		updateTime: newLastModified,
		versionID:  newVersionID,
	}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i interface{}) bool {
	if w.useV2 {
		return errors.As(err, i)
	}
	switch v := err.(type) {
	case awserr.Error:
		if p, ok := i.(*awserr.Error); ok {
			*p = v
			return true
		}
	}
	return false
}

func getErrorCode(err error) string {
	if awsErr, ok := err.(awserr.Error); ok {
		return awsErr.Code()
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

// ErrorCode implements driver.ErrorCode.
func (w *watcher) ErrorCode(err error) gcerrors.ErrorCode {
	code := getErrorCode(err)
	switch code {
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
	return gcerrors.Unknown
}
