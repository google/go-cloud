// Copyright 2019 The Go Cloud Development Kit Authors
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

// Package httpvar provides a runtimevar implementation with variables
// backed by http endpoint. Use New to construct a *runtimevar.Variable.
//
// As
//
// httpvar exposes the following types for As:
//  - Snapshot: *http.Response
//  - Error: *httpvar.RequestError, *url.Error
package httpvar // import "gocloud.dev/runtimevar/httpvar"

import (
	"bytes"
	"context"
	"fmt"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which the HTTP endpoint is called to check for changes.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// RequestError represents an HTTP error that occurred during endpoint call.
type RequestError struct {
	StatusCode int
	URL        string
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("httpvar: received status code %d while calling \"%s\"", e.StatusCode, e.URL)
}

func newRequestError(statusCode int, url string) *RequestError {
	return &RequestError{statusCode, url}
}

// New constructs a *runtimevar.Variable that uses http.Client
// to retrieve the variable contents from endpoint.
func NewVariable(client *http.Client, urlStr string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	endpointURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("httpvar: fail to parse url: %v", err)
	}

	return runtimevar.New(newWatcher(client, endpointURL, decoder, opts)), nil
}

type state struct {
	val        interface{}
	raw        *http.Response
	rawBytes   []byte
	updateTime time.Time
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
	if s.raw == nil {
		return false
	}
	p, ok := i.(**http.Response)
	if !ok {
		return false
	}
	*p = s.raw
	return true
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
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

// equivalentError returns true if err1 and err2 represent an equivalent error;
// i.e., we don't want to return it to the user as a different error.
func equivalentError(err1, err2 error) bool {
	if err1 == err2 || err1.Error() == err2.Error() {
		return true
	}
	var code1, code2 int
	if e, ok := err1.(*RequestError); ok {
		code1 = e.StatusCode
	}
	if e, ok := err2.(*RequestError); ok {
		code2 = e.StatusCode
	}
	return code1 != 0 && code1 == code2
}

// watcher implements driver.Watcher for configurations provided by the Runtime Configurator
// service.
type watcher struct {
	client   *http.Client
	endpoint *url.URL
	decoder  *runtimevar.Decoder
	wait     time.Duration
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	resp, err := w.client.Get(w.endpoint.String())
	if err != nil {
		return errorState(err, prev), w.wait
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := newRequestError(resp.StatusCode, w.endpoint.String())
		return errorState(err, prev), w.wait
	}

	respBodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	// When endpoint returns the same response again, we return nil as state to not trigger variable update.
	if prev != nil && bytes.Equal(respBodyBytes, prev.(*state).rawBytes) {
		return nil, w.wait
	}

	val, err := w.decoder.Decode(respBodyBytes)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	return &state{
		val:        val,
		raw:        resp,
		rawBytes:   respBodyBytes,
		updateTime: time.Now(),
	}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i interface{}) bool {
	switch v := err.(type) {
	case *url.Error:
		if p, ok := i.(*url.Error); ok {
			*p = *v
			return true
		}
	case *RequestError:
		if p, ok := i.(*RequestError); ok {
			*p = *v
			return true
		}
	}
	return false
}

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	if httpErr, ok := err.(*RequestError); ok {
		switch httpErr.StatusCode {
		case http.StatusBadRequest:
			return gcerr.InvalidArgument
		case http.StatusNotFound:
			return gcerr.NotFound
		case http.StatusUnauthorized:
			return gcerr.PermissionDenied
		case http.StatusGatewayTimeout, http.StatusRequestTimeout:
			return gcerr.DeadlineExceeded
		case http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusBadGateway:
			return gcerr.Internal
		}
	}
	return gcerr.Unknown
}

func newWatcher(client *http.Client, endpoint *url.URL, decoder *runtimevar.Decoder, opts *Options) driver.Watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		client:   client,
		endpoint: endpoint,
		decoder:  decoder,
		wait:     driver.WaitDuration(opts.WaitDuration),
	}
}
