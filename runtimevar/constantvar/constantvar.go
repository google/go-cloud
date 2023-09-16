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

// Package constantvar provides a runtimevar implementation with Variables
// that never change. Use New, NewBytes, NewFromEnv, or NewError to construct a
// *runtimevar.Variable.
//
// # URLs
//
// For runtimevar.OpenVariable, constantvar registers for the scheme "constant".
// For more details on the URL format, see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// constantvar does not support any types for As.
package constantvar // import "gocloud.dev/runtimevar/constantvar"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, &URLOpener{})
}

// Scheme is the URL scheme constantvar registers its URLOpener under on blob.DefaultMux.
const Scheme = "constant"

// URLOpener opens constantvar URLs like "constant://?val=foo&decoder=string".
//
// The host and path are ignored.
//
// The following URL parameters are supported:
//   - val: The value to use for the constant Variable. The bytes from val
//     are passed to NewBytes.
//   - envvar: The name of an environment variable to read the value from.
//   - err: The error to use for the constant Variable. A new error is created
//     using errors.New and passed to NewError.
//   - decoder: The decoder to use. Defaults to runtimevar.BytesDecoder.
//     See runtimevar.DecoderByName for supported values.
//
// If multiple of "val", "envvar", or "err" are provided, "err" wins, then "envvar",
// then "val".
type URLOpener struct {
	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder
}

// OpenVariableURL opens the variable at the URL's path. See the package doc
// for more details.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	val := q.Get("val")
	q.Del("val")

	envvar := q.Get("envvar")
	q.Del("envvar")

	errVal := q.Get("err")
	q.Del("err")

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(ctx, decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}
	if errVal != "" {
		return NewError(errors.New(errVal)), nil
	}
	if envvar != "" {
		return NewFromEnv(envvar, decoder), nil
	}
	return NewBytes([]byte(val), decoder), nil
}

var errNotExist = errors.New("variable does not exist")

// New constructs a *runtimevar.Variable holding value.
func New(value interface{}) *runtimevar.Variable {
	return runtimevar.New(&watcher{value: value, t: time.Now()})
}

// NewBytes uses decoder to decode b. If the decode succeeds, it constructs
// a *runtimevar.Variable holding the decoded value. If the decode fails, it
// constructs a runtimevar.Variable that always fails with the error.
func NewBytes(b []byte, decoder *runtimevar.Decoder) *runtimevar.Variable {
	value, err := decoder.Decode(context.Background(), b)
	if err != nil {
		return NewError(err)
	}
	return New(value)
}

// NewFromEnv reads an environment variable and uses decoder to decode it.
// If the decode succeeds, it constructs a *runtimevar.Variable holding the
// decoded value. If the decode fails, it constructs a runtimevar.Variable
// that always fails with the error.
// Note that the value of the constantvar is frozen at initialization time;
// it does not get a new value if the underlying environment variable value
// changes.
func NewFromEnv(envVarName string, decoder *runtimevar.Decoder) *runtimevar.Variable {
	val := os.Getenv(envVarName)
	value, err := decoder.Decode(context.Background(), []byte(val))
	if err != nil {
		return NewError(err)
	}
	return New(value)
}

// NewError constructs a *runtimevar.Variable that always fails. Runtimevar
// wraps errors returned by drivers, so the error returned
// by runtimevar will not equal err.
func NewError(err error) *runtimevar.Variable {
	return runtimevar.New(&watcher{err: err})
}

// watcher implements driver.Watcher and driver.State.
type watcher struct {
	value interface{}
	err   error
	t     time.Time
}

// Value implements driver.State.Value.
func (w *watcher) Value() (interface{}, error) {
	return w.value, w.err
}

// UpdateTime implements driver.State.UpdateTime.
func (w *watcher) UpdateTime() time.Time {
	return w.t
}

// As implements driver.State.As.
func (w *watcher) As(i interface{}) bool {
	return false
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	// The first time this is called, return the constant value.
	if prev == nil {
		return w, 0
	}
	// On subsequent calls, block forever as the value will never change.
	<-ctx.Done()
	w.err = ctx.Err()
	return w, 0
}

// Close implements driver.Close.
func (*watcher) Close() error { return nil }

// ErrorAs implements driver.ErrorAs.
func (*watcher) ErrorAs(err error, i interface{}) bool { return false }

// ErrorCode implements driver.ErrorCode
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	if err == errNotExist {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}
