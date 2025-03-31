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
// and configuration.
//
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// awssecretsmanager exposes the following types for As:
//   - Snapshot: *secretsmanager.GetSecretValueOutput, *secretsmanager.DescribeSecretOutput
//   - Error: any error type returned by the service, notably smithy.APIError
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
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

// Dial gets an AWS secretsmanager service client using the AWS SDK V2.
func Dial(cfg aws.Config) *secretsmanager.Client {
	return secretsmanager.NewFromConfig(cfg)
}

// URLOpener opens AWS Secrets Manager URLs like "awssecretsmanager://my-secret-var-name".
// A friendly name of the secret must be specified. You can NOT specify the Amazon Resource Name (ARN).
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
	cfg, err := gcaws.V2ConfigFromURLParams(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, err)
	}
	return OpenVariable(secretsmanager.NewFromConfig(cfg), path.Join(u.Host, u.Path), decoder, &opts)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which AWS Secrets Manager is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the variable name in AWS Secrets Manager,
// using AWS SDK V2.
// A friendly name of the secret must be specified. You can NOT specify the Amazon Resource Name (ARN).
// Secrets Manager returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(client *secretsmanager.Client, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(client, name, decoder, opts)), nil
}

var OpenVariableV2 = OpenVariable

// state implements driver.State.
type state struct {
	val        any
	rawGet     *secretsmanager.GetSecretValueOutput
	rawDesc    *secretsmanager.DescribeSecretOutput
	updateTime time.Time
	versionID  string
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
	// Map to the more standard context package error.
	if strings.Contains(err.Error(), "context deadline exceeded") {
		err = context.DeadlineExceeded
	} else if getErrorCode(err) == "CancelledError" {
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
	// client is the client to use.
	client *secretsmanager.Client
	// name is an ID of a secret to retrieve.
	name string
	// wait is the amount of time to wait between querying AWS.
	wait time.Duration
	// decoder is the decoder that unmarshalls the value in the param.
	decoder *runtimevar.Decoder
}

func newWatcher(client *secretsmanager.Client, name string, decoder *runtimevar.Decoder, opts *Options) *watcher {
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

func getSecretValue(ctx context.Context, client *secretsmanager.Client, secretID string) (string, []byte, string, *secretsmanager.GetSecretValueOutput, error) {
	getResp, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return "", nil, "", nil, err
	}
	return aws.ToString(getResp.VersionId), getResp.SecretBinary, aws.ToString(getResp.SecretString), getResp, nil
}

func describeSecret(ctx context.Context, client *secretsmanager.Client, secretID string) (time.Time, *secretsmanager.DescribeSecretOutput, error) {
	descResp, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return time.Time{}, nil, err
	}
	return aws.ToTime(descResp.LastChangedDate), descResp, nil
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	var lastVersionID string
	if prev != nil {
		lastVersionID = prev.(*state).versionID
	}

	// GetParameter from S3 to get the current value and version.
	newVersionID, newValBinary, newValString, rawGet, err := getSecretValue(ctx, w.client, w.name)
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
	newLastModified, rawDesc, err := describeSecret(ctx, w.client, w.name)
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
		rawGet:     rawGet,
		rawDesc:    rawDesc,
		updateTime: newLastModified,
		versionID:  newVersionID,
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
	ec, ok := errorCodeMap[code]
	if !ok {
		return gcerrors.Unknown
	}
	return ec
}

var errorCodeMap = map[string]gcerrors.ErrorCode{
	(&types.ResourceNotFoundException{}).ErrorCode():        gcerrors.NotFound,
	(&types.InvalidParameterException{}).ErrorCode():        gcerrors.InvalidArgument,
	(&types.InvalidRequestException{}).ErrorCode():          gcerrors.InvalidArgument,
	(&types.InvalidNextTokenException{}).ErrorCode():        gcerrors.InvalidArgument,
	(&types.EncryptionFailure{}).ErrorCode():                gcerrors.Internal,
	(&types.DecryptionFailure{}).ErrorCode():                gcerrors.Internal,
	(&types.InternalServiceError{}).ErrorCode():             gcerrors.Internal,
	(&types.ResourceExistsException{}).ErrorCode():          gcerrors.AlreadyExists,
	(&types.PreconditionNotMetException{}).ErrorCode():      gcerrors.FailedPrecondition,
	(&types.MalformedPolicyDocumentException{}).ErrorCode(): gcerrors.FailedPrecondition,
	(&types.LimitExceededException{}).ErrorCode():           gcerrors.ResourceExhausted,
}
