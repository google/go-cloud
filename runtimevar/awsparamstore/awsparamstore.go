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
// and configuration.
//
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// awsparamstore exposes the following types for As:
//   - Snapshot: *ssm.GetParameterOutput
//   - Error: any error type returned by the service, notably smithy.APIError
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
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
	Dial,
)

// Dial gets an AWS SSM service client using the AWS SDK V2.
func Dial(cfg aws.Config) *ssm.Client {
	return ssm.NewFromConfig(cfg)
}

// URLOpener opens AWS Paramstore URLs like "awsparamstore://myvar".
//
// See https://pkg.go.dev/gocloud.dev/aws#V2ConfigFromURLParams.
//
// In addition, the following URL parameters are supported:
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//     runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//     See runtimevar.DecoderByName for supported values.
//   - wait: The poll interval, in time.ParseDuration formats.
//     Defaults to 30s.
type URLOpener struct {
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
	opener := &URLOpener{}
	return opener.OpenVariableURL(ctx, u)
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

	cfg, err := gcaws.V2ConfigFromURLParams(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, err)
	}
	return OpenVariable(ssm.NewFromConfig(cfg), path.Join(u.Host, u.Path), decoder, &opts)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which Parameter Store is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the variable name in
// AWS Systems Manager Parameter Store, using AWS SDK V2.
// Parameter Store returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(client *ssm.Client, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(client, name, decoder, opts)), nil
}

var OpenVariableV2 = OpenVariable

func newWatcher(client *ssm.Client, name string, decoder *runtimevar.Decoder, opts *Options) *watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		client:  client,
		name:    name,
		wait:    driver.WaitDuration(opts.WaitDuration),
		decoder: decoder,
	}
}

// state implements driver.State.
type state struct {
	val        any
	rawGet     *ssm.GetParameterOutput
	updateTime time.Time
	version    int64
	err        error
}

// Value implements driver.State.Value.
func (s *state) Value() (any, error) {
	return s.val, s.err
}

// UpdateTime implements driver.State.UpdateTime.
func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// As implements driver.State.As.
func (s *state) As(i any) bool {
	switch p := i.(type) {
	case **ssm.GetParameterOutput:
		*p = s.rawGet
	default:
		return false
	}
	return true
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	// Map aws.RequestCanceled to the more standard context package errors.
	if getErrorCode(err) == "CancelledError" {
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
	// client is the client to use.
	client *ssm.Client
	// name is the parameter to retrieve.
	name string
	// wait is the amount of time to wait between querying AWS.
	wait time.Duration
	// decoder is the decoder that unmarshals the value in the param.
	decoder *runtimevar.Decoder
}

func getParameter(ctx context.Context, client *ssm.Client, name string) (int64, []byte, time.Time, *ssm.GetParameterOutput, error) {
	getResp, err := client.GetParameter(ctx, &ssm.GetParameterInput{
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
	return getResp.Parameter.Version, []byte(aws.ToString(getResp.Parameter.Value)), aws.ToTime(getResp.Parameter.LastModifiedDate), getResp, nil
}

func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	lastVersion := int64(-1)
	if prev != nil {
		lastVersion = prev.(*state).version
	}

	// GetParameter from S3 to get the current value and version.
	newVersion, newVal, newLastModified, rawGet, err := getParameter(ctx, w.client, w.name)
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
		rawGet:     rawGet,
		updateTime: newLastModified,
		version:    newVersion,
	}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i any) bool {
	return errors.As(err, i)
}

func getErrorCode(err error) string {
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
