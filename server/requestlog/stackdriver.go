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
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"time"
)

// A StackdriverLogger writes log entries in the Stackdriver forward JSON
// format.  The record's fields are suitable for consumption by
// Stackdriver Logging.
type StackdriverLogger struct {
	onErr func(error)

	mu  sync.Mutex
	w   io.Writer
	buf bytes.Buffer
	enc *json.Encoder
}

// NewStackdriverLogger returns a new logger that writes to w.
// A nil onErr is treated the same as func(error) {}.
func NewStackdriverLogger(w io.Writer, onErr func(error)) *StackdriverLogger {
	l := &StackdriverLogger{
		w:     w,
		onErr: onErr,
	}
	l.enc = json.NewEncoder(&l.buf)
	return l
}

// Log writes a record to its writer.  Multiple concurrent calls will
// produce sequential writes to its writer.
func (l *StackdriverLogger) Log(ent *Entry) {
	if err := l.log(ent); err != nil && l.onErr != nil {
		l.onErr(err)
	}
}

func (l *StackdriverLogger) log(ent *Entry) error {
	defer l.mu.Unlock()
	l.mu.Lock()

	l.buf.Reset()
	// r represents the fluent-plugin-google-cloud format
	// See https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/f93046d92f7722db2794a042c3f2dde5df91a90b/lib/fluent/plugin/out_google_cloud.rb#L145
	// to check json tags
	var r struct {
		HTTPRequest struct {
			RequestMethod string `json:"requestMethod"`
			RequestURL    string `json:"requestUrl"`
			RequestSize   int64  `json:"requestSize,string"`
			Status        int    `json:"status"`
			ResponseSize  int64  `json:"responseSize,string"`
			UserAgent     string `json:"userAgent"`
			RemoteIP      string `json:"remoteIp"`
			Referer       string `json:"referer"`
			Latency       string `json:"latency"`
		} `json:"httpRequest"`
		Timestamp struct {
			Seconds int64 `json:"seconds"`
			Nanos   int   `json:"nanos"`
		} `json:"timestamp"`
		TraceID string `json:"logging.googleapis.com/trace"`
		SpanID  string `json:"logging.googleapis.com/spanId"`
	}
	r.HTTPRequest.RequestMethod = ent.RequestMethod
	r.HTTPRequest.RequestURL = ent.RequestURL
	// TODO(light): determine whether this is the formula LogEntry expects.
	r.HTTPRequest.RequestSize = ent.RequestHeaderSize + ent.RequestBodySize
	r.HTTPRequest.Status = ent.Status
	// TODO(light): determine whether this is the formula LogEntry expects.
	r.HTTPRequest.ResponseSize = ent.ResponseHeaderSize + ent.ResponseBodySize
	r.HTTPRequest.UserAgent = ent.UserAgent
	r.HTTPRequest.RemoteIP = ent.RemoteIP
	r.HTTPRequest.Referer = ent.Referer
	r.HTTPRequest.Latency = string(appendLatency(nil, ent.Latency))

	t := ent.ReceivedTime.Add(ent.Latency)
	r.Timestamp.Seconds = t.Unix()
	r.Timestamp.Nanos = t.Nanosecond()
	r.TraceID = ent.TraceID.String()
	r.SpanID = ent.SpanID.String()
	if err := l.enc.Encode(r); err != nil {
		return err
	}
	_, err := l.w.Write(l.buf.Bytes())

	return err
}

func appendLatency(b []byte, d time.Duration) []byte {
	// Parses format understood by google-fluentd (which is looser than the documented LogEntry format).
	// See the comment at https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud/blob/e2f60cdd1d97e79ffe4e91bdbf6bd84837f27fa5/lib/fluent/plugin/out_google_cloud.rb#L1539
	b = strconv.AppendFloat(b, d.Seconds(), 'f', 9, 64)
	b = append(b, 's')
	return b
}
