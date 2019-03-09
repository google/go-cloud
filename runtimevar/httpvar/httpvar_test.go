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

package httpvar

import (
	"context"
	"errors"
	"fmt"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
	"gocloud.dev/internal/gcerr"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type harness struct {
	server *httptest.Server
	closeServer func()
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	endpointUrl, err := url.Parse(h.server.URL)
	if err != nil {
		return nil, err
	}
	return newWatcher(http.DefaultClient, endpointUrl, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	_, err := http.Get(fmt.Sprintf("%s/create-variable?value=%s", h.server.URL, url.QueryEscape(string(val))))
	if err != nil {
		return err
	}
	return nil
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	_, err := http.Get(fmt.Sprintf("%s/create-variable?value=%s", h.server.URL, url.QueryEscape(string(val))))
	if err != nil {
		return err
	}
	return nil
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	_, err := http.Get(fmt.Sprintf("%s/delete-variable", h.server.URL))
	if err != nil {
		return err
	}
	return nil
}

func (h *harness) Close() {
	h.closeServer()
}

func (h *harness) Mutable() bool {
	return true
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	server, closeServer := newMockServer()
	return &harness{server: server, closeServer: closeServer }, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var resp *http.Response
	if !s.As(&resp) {
		return errors.New("Snapshot.As failed")
	}

	s2 := state{raw: nil}
	if s2.As(nil) {
		return errors.New("Snapshot.As was expected to fail")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var e RequestError
	if !v.ErrorAs(err, &e) {
		return errors.New("ErrorAs expected to succeed with *httpvar.RequestError")
	}
	if !strings.Contains(e.Error(), e.URL) || !strings.Contains(e.Error(), strconv.Itoa(e.StatusCode)) {
		return errors.New("should contain url and status code")
	}

	var e2 url.Error
	urlError := &url.Error{ URL: "http://example.com", Op: "GET", Err: errors.New("example error") }
	if !v.ErrorAs(urlError, &e2) {
		return errors.New("ErrorAs expected to succeed with *url.Error")
	}

	var e3 RequestError
	if v.ErrorAs(errors.New("example error"), &e3) {
		return errors.New("ErrorAs was expected to fail")
	}
	return nil
}

// httpvar-specific tests.

func TestNewVariable(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		{ "http://example.com/config", false },
		{ "%gh&%ij", true },
	}

	for _, test := range tests {
		_, err := NewVariable(nil, test.URL, runtimevar.StringDecoder, nil)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func TestEquivalentError(t *testing.T) {
	notFoundErr := newRequestError(http.StatusNotFound, "http://example.com")
	badGatewayErr := newRequestError(http.StatusBadGateway, "http://example.com")
	tests := []struct{
		Err1, Err2 error
		Want       bool
	}{
		{Err1: errors.New("error one"), Err2: errors.New("error one"), Want: true},
		{Err1: errors.New("error one"), Err2: errors.New("error two"), Want: false},
		{Err1: errors.New("error one"), Err2: notFoundErr, Want: false},
		{Err1: notFoundErr, Err2: notFoundErr, Want: true},
		{Err1: notFoundErr, Err2: badGatewayErr, Want: false},
	}

	for _, test := range tests {
		got := equivalentError(test.Err1, test.Err2)
		if got != test.Want {
			t.Errorf("%v vs %v: got %v want %v", test.Err1, test.Err2, got, test.Want)
		}
	}
}

func TestWatcher_ErrorCode(t *testing.T) {
	tests := []struct{
		Err   *RequestError
		GCErr gcerr.ErrorCode
	}{
		{ Err: newRequestError(http.StatusBadRequest, ""), GCErr: gcerr.InvalidArgument },
		{ Err: newRequestError(http.StatusNotFound, ""), GCErr: gcerr.NotFound },
		{ Err: newRequestError(http.StatusUnauthorized, ""), GCErr: gcerr.PermissionDenied },
		{ Err: newRequestError(http.StatusGatewayTimeout, ""), GCErr: gcerr.DeadlineExceeded },
		{ Err: newRequestError(http.StatusRequestTimeout, ""), GCErr: gcerr.DeadlineExceeded },
		{ Err: newRequestError(http.StatusInternalServerError, ""), GCErr: gcerr.Internal },
		{ Err: newRequestError(http.StatusServiceUnavailable, ""), GCErr: gcerr.Internal },
		{ Err: newRequestError(http.StatusBadGateway, ""), GCErr: gcerr.Internal },
	}

	endpointUrl, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatal(err)
	}

	watcher := newWatcher(http.DefaultClient, endpointUrl, runtimevar.StringDecoder, nil)
	for _, test := range tests {
		actualGCErr := watcher.ErrorCode(test.Err)
		if test.GCErr != actualGCErr {
			t.Errorf("expected gcerr.ErrorCode to be %d, got %d", test.GCErr, actualGCErr)
		}
	}
}

func TestWatcher_WatchVariable(t *testing.T) {
	t.Run("client returns an error", func(t *testing.T) {
		endpointUrl, err := url.Parse("http://example.com")
		if err != nil {
			t.Fatal(err)
		}

		// In order to force httpClient.Get to return an error, we pass custom *http.Client
		// with every short timeout, so that request will timed out and return an error
		httpClient := &http.Client{
			Timeout: time.Duration(1 * time.Millisecond),
		}
		watcher := newWatcher(httpClient, endpointUrl, runtimevar.StringDecoder, nil)
		state, _ := watcher.WatchVariable(context.Background(), &state{})

		val, err := state.Value()
		if err == nil {
			t.Errorf("expected error got nil")
		}
		if val != nil {
			t.Errorf("expected state value to be nil, got %v", val)
		}
	})
}
