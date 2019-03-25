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

package requestlog

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opencensus.io/trace"
)

func TestHandler(t *testing.T) {
	const requestMsg = "Hello, World!"
	const responseMsg = "I see you."
	const userAgent = "Request Log Test UA"
	const referer = "http://www.example.com/"
	r, err := http.NewRequest("POST", "http://localhost/foo", strings.NewReader(requestMsg))
	if err != nil {
		t.Fatal("NewRequest:", err)
	}
	r.Header.Set("User-Agent", userAgent)
	r.Header.Set("Referer", referer)
	requestHdrSize := len(fmt.Sprintf("User-Agent: %s\r\nReferer: %s\r\nContent-Length: %v\r\n", userAgent, referer, len(requestMsg)))
	responseHdrSize := len(fmt.Sprintf("Content-Length: %v\r\n", len(responseMsg)))
	ent, spanCtx, err := roundTrip(r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(responseMsg)))
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, responseMsg)
	}))
	if err != nil {
		t.Fatal("Could not get entry:", err)
	}
	if want := "POST"; ent.RequestMethod != want {
		t.Errorf("RequestMethod = %q; want %q", ent.RequestMethod, want)
	}
	if want := "/foo"; ent.RequestURL != want {
		t.Errorf("RequestURL = %q; want %q", ent.RequestURL, want)
	}
	if ent.RequestHeaderSize < int64(requestHdrSize) {
		t.Errorf("RequestHeaderSize = %d; want >=%d", ent.RequestHeaderSize, requestHdrSize)
	}
	if ent.RequestBodySize != int64(len(requestMsg)) {
		t.Errorf("RequestBodySize = %d; want %d", ent.RequestBodySize, len(requestMsg))
	}
	if ent.UserAgent != userAgent {
		t.Errorf("UserAgent = %q; want %q", ent.UserAgent, userAgent)
	}
	if ent.Referer != referer {
		t.Errorf("Referer = %q; want %q", ent.Referer, referer)
	}
	if want := "HTTP/1.1"; ent.Proto != want {
		t.Errorf("Proto = %q; want %q", ent.Proto, want)
	}
	if ent.Status != http.StatusOK {
		t.Errorf("Status = %d; want %d", ent.Status, http.StatusOK)
	}
	if ent.ResponseHeaderSize < int64(responseHdrSize) {
		t.Errorf("ResponseHeaderSize = %d; want >=%d", ent.ResponseHeaderSize, responseHdrSize)
	}
	if ent.ResponseBodySize != int64(len(responseMsg)) {
		t.Errorf("ResponseBodySize = %d; want %d", ent.ResponseBodySize, len(responseMsg))
	}
	if ent.TraceID != spanCtx.TraceID {
		t.Errorf("TraceID = %v; want %v", ent.TraceID, spanCtx.TraceID)
	}
	if ent.SpanID != spanCtx.SpanID {
		t.Errorf("SpanID = %v; want %v", ent.SpanID, spanCtx.SpanID)
	}
}

type testSpanHandler struct {
	h       http.Handler
	spanCtx *trace.SpanContext
}

func (sh *testSpanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "test")
	defer span.End()
	r = r.WithContext(ctx)
	sc := trace.FromContext(ctx).SpanContext()
	sh.spanCtx = &sc
	sh.h.ServeHTTP(w, r)
}

func roundTrip(r *http.Request, h http.Handler) (*Entry, *trace.SpanContext, error) {
	capture := new(captureLogger)
	hh := NewHandler(capture, h)
	handler := &testSpanHandler{h: hh}
	s := httptest.NewServer(handler)
	defer s.Close()
	r.URL.Host = s.URL[len("http://"):]
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, nil, err
	}
	resp.Body.Close()
	return &capture.ent, handler.spanCtx, nil
}

type captureLogger struct {
	ent Entry
}

func (cl *captureLogger) Log(ent *Entry) {
	cl.ent = *ent
}
