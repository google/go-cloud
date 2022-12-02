// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package awsparamstore provides a runtimevar implementation with variables
// read from AWS Systems Manager Parameter Store
// (https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-paramstore.html)
// Use OpenVariable to construct a *runtimevar.Variable.
//
// # URLs
//
// For runtimevar.OpenVariable, awsparamstore registers for the scheme "awsparamstore".
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// awsparamstore exposes the following types for As:
//   - Snapshot: (V1) *ssm.GetParameterOutput, (V2) *ssmv2.GetParameterOutput
//   - Error: (V1) awserr.Error, (V2) any error type returned by the service, notably smithy.APIError
package awsparamstore // import "gocloud.dev/runtimevar/awsparamstore"

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
	ssmv2 "github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ssm"
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

// URLOpener opens AWS Paramstore URLs like "awsparamstore://myvar".
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
		o.opener = &URLOpener{ConfigProvider: sess}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, o.err)
	}
	return o.opener.OpenVariableURL(ctx, u)
}

// Scheme is the URL scheme awsparamstore registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "awsparamstore"

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
		return OpenVariableV2(ssmv2.NewFromConfig(cfg), path.Join(u.Host, u.Path), decoder, &opts)
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
	// WaitDuration controls the rate at which Parameter Store is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the variable name in
// AWS Systems Manager Parameter Store.
// Parameter Store returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(sess client.ConfigProvider, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(false, sess, nil, name, decoder, opts)), nil
}

// OpenVariableV2 constructs a *runtimevar.Variable backed by the variable name in
// AWS Systems Manager Parameter Store, using AWS SDK V2.
// Parameter Store returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariableV2(client *ssmv2.Client, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(true, nil, client, name, decoder, opts)), nil
}

func newWatcher(useV2 bool, sess client.ConfigProvider, clientV2 *ssmv2.Client, name string, decoder *runtimevar.Decoder, opts *Options) *watcher {
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

// state implements driver.State.
type state struct {
	val        interface{}
	rawGetV1   *ssm.GetParameterOutput
	rawGetV2   *ssmv2.GetParameterOutput
	updateTime time.Time
	version    int64
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
	case **ssm.GetParameterOutput:
		*p = s.rawGetV1
	case **ssmv2.GetParameterOutput:
		*p = s.rawGetV2
	default:
		return false
	}
	return true
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	// Map aws.RequestCanceled to the more standard context package errors.
	if getErrorCode(err) == request.CanceledErrorCode {
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
	clientV2 *ssmv2.Client
	// name is the parameter to retrieve.
	name string
	// wait is the amount of time to wait between querying AWS.
	wait time.Duration
	// decoder is the decoder that unmarshals the value in the param.
	decoder *runtimevar.Decoder
}

func getParameter(svc *ssm.SSM, name string) (int64, []byte, time.Time, *ssm.GetParameterOutput, error) {
	getResp, err := svc.GetParameter(&ssm.GetParameterInput{
		Name: aws.String(name),
		// Ignored if the parameter is not encrypted.
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return 0, nil, time.Time{}, nil, err
	}
	if getResp.Parameter == nil {
		return 0, nil, time.Time{}, getResp, fmt.Errorf("unable to get %q parameter", name)
	}
	return aws.Int64Value(getResp.Parameter.Version), []byte(aws.StringValue(getResp.Parameter.Value)), aws.TimeValue(getResp.Parameter.LastModifiedDate), getResp, nil
}

func getParameterV2(ctx context.Context, client *ssmv2.Client, name string) (int64, []byte, time.Time, *ssmv2.GetParameterOutput, error) {
	getResp, err := client.GetParameter(ctx, &ssmv2.GetParameterInput{
		Name: aws.String(name),
		// Ignored if the parameter is not encrypted.
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return 0, nil, time.Time{}, nil, err
	}
	if getResp.Parameter == nil {
		return 0, nil, time.Time{}, getResp, fmt.Errorf("unable to get %q parameter", name)
	}
	return getResp.Parameter.Version, []byte(awsv2.ToString(getResp.Parameter.Value)), awsv2.ToTime(getResp.Parameter.LastModifiedDate), getResp, nil
}

func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	lastVersion := int64(-1)
	if prev != nil {
		lastVersion = prev.(*state).version
	}
	var svc *ssm.SSM
	if !w.useV2 {
		svc = ssm.New(w.sess)
	}

	// GetParameter from S3 to get the current value and version.
	var newVersion int64
	var newVal []byte
	var newLastModified time.Time
	var rawGetV1 *ssm.GetParameterOutput
	var rawGetV2 *ssmv2.GetParameterOutput
	var err error
	if w.useV2 {
		newVersion, newVal, newLastModified, rawGetV2, err = getParameterV2(ctx, w.clientV2, w.name)
	} else {
		newVersion, newVal, newLastModified, rawGetV1, err = getParameter(svc, w.name)
	}
	if err != nil {
		return errorState(err, prev), w.wait
	}
	if newVersion == lastVersion {
		// Version hasn't changed, so no change; return nil.
		return nil, w.wait
	}

	// New value (or at least, new version). Decode it.
	val, err := w.decoder.Decode(ctx, newVal)
	if err != nil {
		return errorState(err, prev), w.wait
	}
	return &state{
		val:        val,
		rawGetV1:   rawGetV1,
		rawGetV2:   rawGetV2,
		updateTime: newLastModified,
		version:    newVersion,
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
	if code == "ParameterNotFound" {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}
