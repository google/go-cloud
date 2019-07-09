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
	"testing"
	"time"
)

var _ Logger = (*NCSALogger)(nil)

func TestNCSALog(t *testing.T) {
	const (
		startTime      = 1507914000
		startTimeNanos = 512

		latencySec   = 5
		latencyNanos = 123456789

		endTime      = startTime + latencySec
		endTimeNanos = startTimeNanos + latencyNanos
	)
	tests := []struct {
		name string
		ent  Entry
		want string
	}{
		{
			name: "AllFields",
			ent: Entry{
				ReceivedTime:       time.Unix(startTime, startTimeNanos).UTC(),
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
			},
			want: `12.34.56.78 - - [13/Oct/2017:17:00:00 +0000] "POST /foo/bar HTTP/1.1" 404 789000 "http://www.example.com/" "Chrome proxied through Firefox and Edge"` + "\n",
		},
		{
			name: "OnlyRequiredFields",
			ent: Entry{
				ReceivedTime:  time.Unix(startTime, startTimeNanos).UTC(),
				RequestMethod: "POST",
				RequestURL:    "/foo/bar",
				Proto:         "HTTP/1.1",
				Status:        404,
			},
			want: `- - - [13/Oct/2017:17:00:00 +0000] "POST /foo/bar HTTP/1.1" 404 0 "" ""` + "\n",
		},
		{
			name: "OnlyRequiredFieldsAndUserAgent",
			ent: Entry{
				ReceivedTime:  time.Unix(startTime, startTimeNanos).UTC(),
				RequestMethod: "POST",
				RequestURL:    "/foo/bar",
				Proto:         "HTTP/1.1",
				Status:        404,
				UserAgent:     "Chrome proxied through Firefox and Edge",
			},
			want: `- - - [13/Oct/2017:17:00:00 +0000] "POST /foo/bar HTTP/1.1" 404 0 "" "Chrome proxied through Firefox and Edge"` + "\n",
		},
		{
			name: "DoubleQuotesInUserAgent",
			ent: Entry{
				ReceivedTime:  time.Unix(startTime, startTimeNanos).UTC(),
				RequestMethod: "POST",
				RequestURL:    "/foo/bar",
				Proto:         "HTTP/1.1",
				Status:        404,
				UserAgent:     "Chrome \"proxied\" through Firefox and Edge",
			},
			want: `- - - [13/Oct/2017:17:00:00 +0000] "POST /foo/bar HTTP/1.1" 404 0 "" "Chrome \"proxied\" through Firefox and Edge"` + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			var logErr error
			l := NewNCSALogger(buf, func(e error) { logErr = e })
			l.Log(&test.ent)
			if logErr != nil {
				t.Error("Logger called error callback:", logErr)
			}
			got := buf.String()
			if got != test.want {
				t.Errorf("Log(...) wrote %q; want %q", got, test.want)
			}
		})
	}
}
