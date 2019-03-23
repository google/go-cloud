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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opencensus.io/trace"
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
	ctx, span := trace.StartSpan(context.Background(), "test")
	defer span.End()
	sc := trace.FromContext(ctx).SpanContext()
	buf := new(bytes.Buffer)
	var logErr error
	l := NewStackdriverLogger(buf, func(e error) { logErr = e })
	want := &Entry{
		ReceivedTime:       time.Unix(startTime, startTimeNanos),
		RequestMethod:      "POST",
		RequestURL:         "/foo/bar",
		RequestHeaderSize:  456,
		RequestBodySize:    123000,
		UserAgent:          "Chrome proxied through Firefox and Edge",
		Referer:            "http://www.example.com/",
		Proto:              "HTTP/1.1",
		RemoteIP:           "12.34.56.78",
		ServerIP:           "127.0.0.1",
		Status:             404,
		ResponseHeaderSize: 555,
		ResponseBodySize:   789000,
		Latency:            latencySec*time.Second + latencyNanos*time.Nanosecond,
		TraceID:            sc.TraceID,
		SpanID:             sc.SpanID,
	}
	ent := *want // copy in case Log accidentally mutates
	l.Log(&ent)
	if logErr != nil {
		t.Error("Logger called error callback:", logErr)
	}

	var got json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal("Unmarshal:", err)
	}

	var r map[string]interface{}
	if err := json.Unmarshal(got, &r); err != nil {
		t.Error("Unmarshal record:", err)
	} else {
		rr, _ := r["httpRequest"].(map[string]interface{})
		if rr == nil {
			t.Error("httpRequest does not exist in record or is not a JSON object")
		}
		if got, want := jsonString(rr, "requestMethod"), ent.RequestMethod; got != want {
			t.Errorf("httpRequest.requestMethod = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "requestUrl"), ent.RequestURL; got != want {
			t.Errorf("httpRequest.requestUrl = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "requestSize"), "123456"; got != want {
			t.Errorf("httpRequest.requestSize = %q; want %q", got, want)
		}
		if got, want := jsonNumber(rr, "status"), float64(ent.Status); got != want {
			t.Errorf("httpRequest.status = %d; want %d", int64(got), int64(want))
		}
		if got, want := jsonString(rr, "responseSize"), "789555"; got != want {
			t.Errorf("httpRequest.responseSize = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "userAgent"), ent.UserAgent; got != want {
			t.Errorf("httpRequest.userAgent = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "remoteIp"), ent.RemoteIP; got != want {
			t.Errorf("httpRequest.remoteIp = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "referer"), ent.Referer; got != want {
			t.Errorf("httpRequest.referer = %q; want %q", got, want)
		}
		if got, want := jsonString(rr, "latency"), "5.123456789"; parseLatency(got) != want {
			t.Errorf("httpRequest.latency = %q; want %q", got, want+"s")
		}
		ts, _ := r["timestamp"].(map[string]interface{})
		if ts == nil {
			t.Error("timestamp does not exist in record or is not a JSON object")
		}
		if got, want := jsonNumber(ts, "seconds"), float64(endTime); got != want {
			t.Errorf("timestamp.seconds = %g; want %g", got, want)
		}
		if got, want := jsonNumber(ts, "nanos"), float64(endTimeNanos); got != want {
			t.Errorf("timestamp.nanos = %g; want %g", got, want)
		}
		if got, want := jsonString(r, "logging.googleapis.com/trace"), ent.TraceID.String(); got != want {
			t.Errorf("traceID = %q; want %q", got, want)
		}
		if got, want := jsonString(r, "logging.googleapis.com/spanId"), ent.SpanID.String(); got != want {
			t.Errorf("spanID = %q; want %q", got, want)
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

func jsonString(obj map[string]interface{}, k string) string {
	v, _ := obj[k].(string)
	return v
}

func jsonNumber(obj map[string]interface{}, k string) float64 {
	v, _ := obj[k].(float64)
	return v
}

func BenchmarkStackdriverLog(b *testing.B) {
	ent := &Entry{
		ReceivedTime:       time.Date(2017, time.October, 13, 17, 0, 0, 512, time.UTC),
		RequestMethod:      "POST",
		RequestURL:         "/foo/bar",
		RequestHeaderSize:  456,
		RequestBodySize:    123000,
		UserAgent:          "Chrome proxied through Firefox and Edge",
		Referer:            "http://www.example.com/",
		Proto:              "HTTP/1.1",
		RemoteIP:           "12.34.56.78",
		ServerIP:           "127.0.0.1",
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

	l = NewStackdriverLogger(ioutil.Discard, func(error) {})
	for i := 0; i < b.N; i++ {
		l.Log(ent)
	}
}

func BenchmarkE2E(b *testing.B) {
	run := func(b *testing.B, handler http.Handler) {
		s := httptest.NewServer(handler)
		defer s.Close()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resp, err := http.Get(s.URL)
			if err != nil {
				b.Fatal(err)
			}
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	b.Run("Baseline", func(b *testing.B) {
		run(b, http.HandlerFunc(benchHandler))
	})
	b.Run("WithLog", func(b *testing.B) {
		l := NewStackdriverLogger(ioutil.Discard, func(error) {})
		run(b, NewHandler(l, http.HandlerFunc(benchHandler)))
	})
}

func benchHandler(w http.ResponseWriter, r *http.Request) {
	const msg = "Hello, World!"
	w.Header().Set("Content-Length", fmt.Sprint(len(msg)))
	io.WriteString(w, msg)
}
