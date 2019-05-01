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

package httpvar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

type harness struct {
	mockServer *mockServer
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	endpointURL, err := url.Parse(h.mockServer.baseURL + "/" + name)
	if err != nil {
		return nil, err
	}
	return newWatcher(http.DefaultClient, endpointURL, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	h.mockServer.SetResponse(name, string(val))
	return nil
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	h.mockServer.SetResponse(name, string(val))
	return nil
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	h.mockServer.DeleteResponse(name)
	return nil
}

func (h *harness) Close() {
	h.mockServer.close()
}

func (h *harness) Mutable() bool {
	return true
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	return &harness{
		mockServer: newMockServer(),
	}, nil
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
	if !strings.Contains(e.Error(), strconv.Itoa(e.Response.StatusCode)) {
		return errors.New("should contain url and status code")
	}

	var e2 url.Error
	urlError := &url.Error{URL: "http://example.com", Op: "GET", Err: errors.New("example error")}
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

func TestOpenVariable(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		{"http://example.com/config", false},
		{"%gh&%ij", true},
	}

	for _, test := range tests {
		v, err := OpenVariable(http.DefaultClient, test.URL, runtimevar.StringDecoder, nil)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if v != nil {
			v.Close()
		}
	}
}

func TestEquivalentError(t *testing.T) {
	notFoundErr := newRequestError(&http.Response{StatusCode: http.StatusNotFound})
	badGatewayErr := newRequestError(&http.Response{StatusCode: http.StatusBadGateway})
	tests := []struct {
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
	tests := []struct {
		Err   *RequestError
		GCErr gcerr.ErrorCode
	}{
		{Err: newRequestError(&http.Response{StatusCode: http.StatusBadRequest}), GCErr: gcerr.InvalidArgument},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusNotFound}), GCErr: gcerr.NotFound},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusUnauthorized}), GCErr: gcerr.PermissionDenied},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusGatewayTimeout}), GCErr: gcerr.DeadlineExceeded},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusRequestTimeout}), GCErr: gcerr.DeadlineExceeded},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusInternalServerError}), GCErr: gcerr.Internal},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusServiceUnavailable}), GCErr: gcerr.Internal},
		{Err: newRequestError(&http.Response{StatusCode: http.StatusBadGateway}), GCErr: gcerr.Internal},
	}

	endpointURL, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatal(err)
	}

	watcher := newWatcher(http.DefaultClient, endpointURL, runtimevar.StringDecoder, nil)
	defer watcher.Close()
	for _, test := range tests {
		actualGCErr := watcher.ErrorCode(test.Err)
		if test.GCErr != actualGCErr {
			t.Errorf("expected gcerr.ErrorCode to be %d, got %d", test.GCErr, actualGCErr)
		}
	}
}

func TestWatcher_WatchVariable(t *testing.T) {
	t.Run("client returns an error", func(t *testing.T) {
		endpointURL, err := url.Parse("http://example.com")
		if err != nil {
			t.Fatal(err)
		}

		// In order to force httpClient.Get to return an error, we pass custom *http.Client
		// with every short timeout, so that request will timed out and return an error.
		httpClient := &http.Client{
			Timeout: time.Duration(1 * time.Millisecond),
		}
		watcher := newWatcher(httpClient, endpointURL, runtimevar.StringDecoder, nil)
		defer watcher.Close()
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

func TestOpenVariableURL(t *testing.T) {
	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	baseURL := h.(*harness).mockServer.baseURL

	ctx := context.Background()
	if err := h.CreateVariable(ctx, "string-var", []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := h.CreateVariable(ctx, "json-var", []byte(`{"Foo": "Bar"}`)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		URL          string
		WantErr      bool
		WantWatchErr bool
		Want         interface{}
	}{
		// Nonexistentvar does not exist, so we get an error from Watch.
		{baseURL + "/nonexistentvar", false, true, nil},
		// Invalid decoder arg.
		{baseURL + "/string-var?decoder=notadecoder", true, false, nil},
		// Working example with string decoder.
		{baseURL + "/string-var?decoder=string", false, false, "hello world"},
		// Working example with default decoder.
		{baseURL + "/string-var", false, false, []byte("hello world")},
		// Working example with JSON decoder.
		{baseURL + "/json-var?decoder=jsonmap", false, false, &map[string]interface{}{"Foo": "Bar"}},
	}

	for _, test := range tests {
		t.Run(test.URL, func(t *testing.T) {
			v, err := runtimevar.OpenVariable(ctx, test.URL)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
			}
			if err != nil {
				return
			}
			defer v.Close()
			snapshot, err := v.Watch(ctx)
			if (err != nil) != test.WantWatchErr {
				t.Errorf("%s: got Watch error %v, want error %v", test.URL, err, test.WantWatchErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(snapshot.Value, test.Want) {
				t.Errorf("%s: got snapshot value\n%v\n  want\n%v", test.URL, snapshot.Value, test.Want)
			}
		})
	}
}

type mockServer struct {
	baseURL   string
	close     func()
	responses map[string]interface{}
}

func (m *mockServer) SetResponse(name string, response interface{}) {
	m.responses[name] = response
}

func (m *mockServer) DeleteResponse(name string) {
	delete(m.responses, name)
}

func newMockServer() *mockServer {
	mock := &mockServer{responses: map[string]interface{}{}}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resp := mock.responses[strings.TrimPrefix(r.URL.String(), "/")]
		if resp == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, resp)
	})

	server := httptest.NewServer(mux)
	mock.baseURL = server.URL
	mock.close = server.Close
	return mock
}
