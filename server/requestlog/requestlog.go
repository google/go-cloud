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

// Package requestlog provides an http.Handler that logs information
// about requests.
package requestlog // import "gocloud.dev/server/requestlog"

import (
	"bufio"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"go.opencensus.io/trace"
)

// Logger wraps the Log method.  Log must be safe to call from multiple
// goroutines.  Log must not hold onto an Entry after it returns.
type Logger interface {
	Log(*Entry)
}

// A Handler emits request information to a Logger.
type Handler struct {
	log Logger
	h   http.Handler
}

// NewHandler returns a handler that emits information to log and calls
// h.ServeHTTP.
func NewHandler(log Logger, h http.Handler) *Handler {
	return &Handler{
		log: log,
		h:   h,
	}
}

// ServeHTTP calls its underlying handler's ServeHTTP method, then calls
// Log after the handler returns.
//
// ServeHTTP will always consume the request body up to the first error,
// even if the underlying handler does not.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sc := trace.FromContext(r.Context()).SpanContext()
	ent := &Entry{
		ReceivedTime:      start,
		RequestMethod:     r.Method,
		RequestURL:        r.URL.String(),
		RequestHeaderSize: headerSize(r.Header),
		UserAgent:         r.UserAgent(),
		Referer:           r.Referer(),
		Proto:             r.Proto,
		RemoteIP:          ipFromHostPort(r.RemoteAddr),
		TraceID:           sc.TraceID,
		SpanID:            sc.SpanID,
	}
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		ent.ServerIP = ipFromHostPort(addr.String())
	}
	r2 := new(http.Request)
	*r2 = *r
	rcc := &readCounterCloser{r: r.Body}
	r2.Body = rcc
	w2 := &responseStats{w: w}

	h.h.ServeHTTP(w2, r2)

	ent.Latency = time.Since(start)
	if rcc.err == nil && rcc.r != nil && !w2.hijacked {
		// If the handler hasn't encountered an error in the Body (like EOF),
		// then consume the rest of the Body to provide an accurate rcc.n.
		io.Copy(ioutil.Discard, rcc)
	}
	ent.RequestBodySize = rcc.n
	ent.Status = w2.code
	if ent.Status == 0 {
		ent.Status = http.StatusOK
	}
	ent.ResponseHeaderSize, ent.ResponseBodySize = w2.size()
	h.log.Log(ent)
}

// Entry records information about a completed HTTP request.
type Entry struct {
	ReceivedTime      time.Time
	RequestMethod     string
	RequestURL        string
	RequestHeaderSize int64
	RequestBodySize   int64
	UserAgent         string
	Referer           string
	Proto             string

	RemoteIP string
	ServerIP string

	Status             int
	ResponseHeaderSize int64
	ResponseBodySize   int64
	Latency            time.Duration
	TraceID            trace.TraceID
	SpanID             trace.SpanID
}

func ipFromHostPort(hp string) string {
	h, _, err := net.SplitHostPort(hp)
	if err != nil {
		return ""
	}
	if len(h) > 0 && h[0] == '[' {
		return h[1 : len(h)-1]
	}
	return h
}

type readCounterCloser struct {
	r   io.ReadCloser
	n   int64
	err error
}

func (rcc *readCounterCloser) Read(p []byte) (n int, err error) {
	if rcc.err != nil {
		return 0, rcc.err
	}
	n, rcc.err = rcc.r.Read(p)
	rcc.n += int64(n)
	return n, rcc.err
}

func (rcc *readCounterCloser) Close() error {
	rcc.err = errors.New("read from closed reader")
	return rcc.r.Close()
}

type writeCounter int64

func (wc *writeCounter) Write(p []byte) (n int, err error) {
	*wc += writeCounter(len(p))
	return len(p), nil
}

func headerSize(h http.Header) int64 {
	var wc writeCounter
	h.Write(&wc)
	return int64(wc) + 2 // for CRLF
}

type responseStats struct {
	w        http.ResponseWriter
	hsize    int64
	wc       writeCounter
	code     int
	hijacked bool
}

func (r *responseStats) Header() http.Header {
	return r.w.Header()
}

func (r *responseStats) WriteHeader(statusCode int) {
	if r.code != 0 {
		return
	}
	r.hsize = headerSize(r.w.Header())
	r.w.WriteHeader(statusCode)
	r.code = statusCode
}

func (r *responseStats) Write(p []byte) (n int, err error) {
	if r.code == 0 {
		r.WriteHeader(http.StatusOK)
	}
	n, err = r.w.Write(p)
	r.wc.Write(p[:n])
	return
}

func (r *responseStats) size() (hdr, body int64) {
	if r.code == 0 {
		return headerSize(r.w.Header()), 0
	}
	// Use the header size from the time WriteHeader was called.
	// The Header map can be mutated after the call to add HTTP Trailers,
	// which we don't want to count.
	return r.hsize, int64(r.wc)
}

func (r *responseStats) Hijack() (_ net.Conn, _ *bufio.ReadWriter, err error) {
	defer func() {
		if err == nil {
			r.hijacked = true
		}
	}()
	if hj, ok := r.w.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
}
