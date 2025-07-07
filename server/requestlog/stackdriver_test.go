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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

func TestStackdriverLog(t *testing.T) {
	const (
		startTime      = 1507914000
		startTimeNanos = 512

		latencySec   = 5
		latencyNanos = 123456789

		endTime      = startTime + latencySec
		endTimeNanos = startTimeNanos + latencyNanos
	)
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test")
	defer span.End()
	sc := trace.SpanContextFromContext(ctx)
	buf := new(bytes.Buffer)
	var logErr error
	l := NewStackdriverLogger(buf, func(e error) { logErr = e })
	wantEntry := &Entry{
		ReceivedTime: time.Unix(startTime, startTimeNanos),
		Request: &http.Request{
			Method: "POST",
			Proto:  "HTTP/1.1",
			Header: http.Header{
				"User-Agent": []string{"Chrome proxied through Firefox and Edge"},
				"Referer":    []string{"http://www.example.com/"},
			},
			Host:       "127.0.0.1",
			RemoteAddr: "12.34.56.78:80",
			RequestURI: "/foo/bar",
		},
		RequestBodySize:    123000,
		Status:             404,
		ResponseHeaderSize: 555,
		ResponseBodySize:   789000,
		Latency:            latencySec*time.Second + latencyNanos*time.Nanosecond,
		TraceID:            sc.TraceID(),
		SpanID:             sc.SpanID(),
	}
	ent := *wantEntry // copy in case Log accidentally mutates
	l.Log(&ent)
	if logErr != nil {
		t.Error("Logger called error callback:", logErr)
	}

	var gotBytes json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &gotBytes); err != nil {
		t.Fatal("Unmarshal:", err)
	}

	var r map[string]any
	if err := json.Unmarshal(gotBytes, &r); err != nil {
		t.Error("Unmarshal record:", err)
	} else {
		rr, _ := r["httpRequest"].(map[string]any)
		if rr == nil {
			t.Error("httpRequest does not exist in record or is not a JSON object")
		}
		entReq := ent.Request
		if got, want := jsonString(rr, "requestMethod"), entReq.Method; got != want {
			t.Errorf("httpRequest.requestMethod = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(rr, "requestUrl"), entReq.RequestURI; got != want {
			t.Errorf("httpRequest.requestUrl = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(rr, "requestSize"), "123089"; got != want {
			t.Errorf("httpRequest.requestSize = %q; wantEntry %q", got, want)
		}
		if got, want := jsonNumber(rr, "status"), float64(ent.Status); got != want {
			t.Errorf("httpRequest.status = %d; wantEntry %d", int64(got), int64(want))
		}
		if got, want := jsonString(rr, "responseSize"), "789555"; got != want {
			t.Errorf("httpRequest.responseSize = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(rr, "userAgent"), entReq.UserAgent(); got != want {
			t.Errorf("httpRequest.userAgent = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(rr, "remoteIp"), ipFromHostPort(entReq.RemoteAddr); got != want {
			t.Errorf("httpRequest.remoteIp = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(rr, "referer"), entReq.Referer(); got != want {
			t.Errorf("httpRequest.referer = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(rr, "latency"), "5.123456789"; parseLatency(got) != want {
			t.Errorf("httpRequest.latency = %q; wantEntry %q", got, want+"s")
		}
		ts, _ := r["timestamp"].(map[string]any)
		if ts == nil {
			t.Error("timestamp does not exist in record or is not a JSON object")
		}
		if got, want := jsonNumber(ts, "seconds"), float64(endTime); got != want {
			t.Errorf("timestamp.seconds = %g; wantEntry %g", got, want)
		}
		if got, want := jsonNumber(ts, "nanos"), float64(endTimeNanos); got != want {
			t.Errorf("timestamp.nanos = %g; wantEntry %g", got, want)
		}
		if got, want := jsonString(r, "logging.googleapis.com/trace"), ent.TraceID.String(); got != want {
			t.Errorf("traceID = %q; wantEntry %q", got, want)
		}
		if got, want := jsonString(r, "logging.googleapis.com/spanId"), ent.SpanID.String(); got != want {
			t.Errorf("spanID = %q; wantEntry %q", got, want)
		}
	}
}

func parseLatency(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "s") {
		return ""
	}
	s = strings.TrimSpace(s[:len(s)-1])
	for _, c := range s {
		if !(c >= '0' && c <= '9') && c != '.' {
			return ""
		}
	}
	return s
}

func jsonString(obj map[string]any, k string) string {
	v, _ := obj[k].(string)
	return v
}

func jsonNumber(obj map[string]any, k string) float64 {
	v, _ := obj[k].(float64)
	return v
}

func BenchmarkStackdriverLog(b *testing.B) {
	ent := &Entry{
		ReceivedTime: time.Date(2017, time.October, 13, 17, 0, 0, 512, time.UTC),
		Request: &http.Request{
			Method: "POST",
			Proto:  "HTTP/1.1",
			Header: http.Header{
				"User-Agent": []string{"Chrome proxied through Firefox and Edge"},
				"Referer":    []string{"http://www.example.com/"},
			},
			Host:       "127.0.0.1",
			RemoteAddr: "12.34.56.78",
			RequestURI: "/foo/bar",
		},
		RequestBodySize:    123000,
		Status:             404,
		ResponseHeaderSize: 555,
		ResponseBodySize:   789000,
		Latency:            5 * time.Second,
	}
	var buf bytes.Buffer
	l := NewStackdriverLogger(&buf, func(error) {})
	l.Log(ent)
	b.ReportAllocs()
	b.SetBytes(int64(buf.Len()))
	buf.Reset()
	b.ResetTimer()

	l = NewStackdriverLogger(io.Discard, func(error) {})
	for i := 0; i < b.N; i++ {
		l.Log(ent)
	}
}

func BenchmarkE2E(b *testing.B) {
	run := func(b *testing.B, handler http.Handler) {
		b.Helper()

		s := httptest.NewServer(handler)
		defer s.Close()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resp, err := http.Get(s.URL)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
	b.Run("Baseline", func(b *testing.B) {
		run(b, http.HandlerFunc(benchHandler))
	})
	b.Run("WithLog", func(b *testing.B) {
		l := NewStackdriverLogger(io.Discard, func(error) {})
		run(b, NewHandler(l, http.HandlerFunc(benchHandler)))
	})
}

func benchHandler(w http.ResponseWriter, r *http.Request) {
	const msg = "Hello, World!"
	w.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	_, _ = io.WriteString(w, msg)
}
