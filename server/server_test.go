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

	"gocloud.dev/requestlog"
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
	if td.handler == nil {
		t.Error("testDriver must set handler, got nil")
	}
}

func TestMiddleware(t *testing.T) {
	onLogCalled := false

	tl := &testLogger{
		onLog: func(ent *requestlog.Entry) {
			onLogCalled = true
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
	if !onLogCalled {
		t.Fatal("logging middleware was not called")
	}
}

type testDriver struct {
	listenAndServeCalled bool
	handler              http.Handler
}

func (td *testDriver) ListenAndServe(addr string, h http.Handler) error {
	td.listenAndServeCalled = true
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
