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

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gocloud.dev/server/requestlog"
)

const (
	certFile = "my-cert"
	keyFile  = "my-key"
)

func TestListenAndServe(t *testing.T) {
	td := new(testDriver)
	s := New(http.NotFoundHandler(), &Options{Driver: td})
	err := s.ListenAndServe(":8080")
	if err != nil {
		t.Fatal(err)
	}
	if !td.listenAndServeCalled {
		t.Error("ListenAndServe was not called from the supplied driver")
	}
	if td.certFile != "" || td.keyFile != "" {
		t.Errorf("ListenAndServe got non-empty certFile or keyFile (%q, %q), wanted empty", td.certFile, td.keyFile)
	}
	if td.handler == nil {
		t.Error("testDriver must set handler, got nil")
	}
}

func TestListenAndServeTLSNoSupported(t *testing.T) {
	td := new(testDriverNoTLS)
	s := New(http.NotFoundHandler(), &Options{Driver: td})
	err := s.ListenAndServeTLS(":8080", certFile, keyFile)
	if err == nil {
		t.Fatal("expected TLS not supported error")
	}
}

func TestListenAndServeTLS(t *testing.T) {
	td := new(testDriver)
	s := New(http.NotFoundHandler(), &Options{Driver: td})
	err := s.ListenAndServeTLS(":8080", certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if !td.listenAndServeCalled {
		t.Error("ListenAndServe was not called from the supplied driver")
	}
	if td.certFile != certFile {
		t.Errorf("ListenAndServe got certFile %q, want %q", td.certFile, certFile)
	}
	if td.keyFile != keyFile {
		t.Errorf("ListenAndServe got keyFile %q, want %q", td.keyFile, keyFile)
	}
	if td.handler == nil {
		t.Error("testDriver must set handler, got nil")
	}
}

func TestMiddleware(t *testing.T) {
	onLogCalled := 0

	tl := &testLogger{
		onLog: func(ent *requestlog.Entry) {
			onLogCalled++
			if ent.TraceID.String() == "" {
				t.Error("TraceID is empty")
			}
			if ent.SpanID.String() == "" {
				t.Error("SpanID is empty")
			}
		},
	}

	td := new(testDriver)
	s := New(http.NotFoundHandler(), &Options{Driver: td, RequestLogger: tl})
	err := s.ListenAndServe(":8080")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	td.handler.ServeHTTP(rr, req)
	if onLogCalled != 1 {
		t.Fatal("logging middleware was not called")
	}

	// Repeat with TLS.
	err = s.ListenAndServeTLS(":8081", certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}

	req, err = http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	td.handler.ServeHTTP(rr, req)
	if onLogCalled != 2 {
		t.Fatal("logging middleware was not called for TLS")
	}

}

type testDriverNoTLS string

func (td *testDriverNoTLS) ListenAndServe(addr string, h http.Handler) error {
	return errors.New("this is a method for satisfying the interface")
}

func (td *testDriverNoTLS) Shutdown(ctx context.Context) error {
	return errors.New("this is a method for satisfying the interface")
}

type testDriver struct {
	listenAndServeCalled bool
	certFile, keyFile    string
	handler              http.Handler
}

func (td *testDriver) ListenAndServe(addr string, h http.Handler) error {
	td.listenAndServeCalled = true
	td.handler = h
	return nil
}

func (td *testDriver) ListenAndServeTLS(addr, certFile, keyFile string, h http.Handler) error {
	td.listenAndServeCalled = true
	td.certFile = certFile
	td.keyFile = keyFile
	td.handler = h
	return nil
}

func (td *testDriver) Shutdown(ctx context.Context) error {
	return errors.New("this is a method for satisfying the interface")
}

type testLogger struct {
	onLog func(ent *requestlog.Entry)
}

func (tl *testLogger) Log(ent *requestlog.Entry) {
	tl.onLog(ent)
}
