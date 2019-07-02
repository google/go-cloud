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
	"io"
	"strconv"
	"sync"
)

// An NCSALogger writes log entries to an io.Writer in the
// Combined Log Format.
//
// Details at http://httpd.apache.org/docs/current/logs.html#combined
type NCSALogger struct {
	onErr func(error)

	mu  sync.Mutex
	w   io.Writer
	buf []byte
}

// NewNCSALogger returns a new logger that writes to w.
// A nil onErr is treated the same as func(error) {}.
func NewNCSALogger(w io.Writer, onErr func(error)) *NCSALogger {
	return &NCSALogger{
		w:     w,
		onErr: onErr,
	}
}

// Log writes an entry line to its writer.  Multiple concurrent calls
// will produce sequential writes to its writer.
func (l *NCSALogger) Log(ent *Entry) {
	if err := l.log(ent); err != nil && l.onErr != nil {
		l.onErr(err)
	}
}

func (l *NCSALogger) log(ent *Entry) error {
	defer l.mu.Unlock()
	l.mu.Lock()
	l.buf = formatEntry(l.buf[:0], ent)
	_, err := l.w.Write(l.buf)
	return err
}

func formatEntry(b []byte, ent *Entry) []byte {
	const ncsaTime = "02/Jan/2006:15:04:05 -0700"
	if ent.RemoteIP == "" {
		b = append(b, '-')
	} else {
		b = append(b, ent.RemoteIP...)
	}
	b = append(b, " - - ["...)
	b = ent.ReceivedTime.AppendFormat(b, ncsaTime)
	b = append(b, "] \""...)
	b = append(b, ent.RequestMethod...)
	b = append(b, ' ')
	b = append(b, ent.RequestURL...)
	b = append(b, ' ')
	b = append(b, ent.Proto...)
	b = append(b, "\" "...)
	b = strconv.AppendInt(b, int64(ent.Status), 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, int64(ent.ResponseBodySize), 10)
	b = append(b, ' ')
	b = strconv.AppendQuote(b, ent.Referer)
	b = append(b, ' ')
	b = strconv.AppendQuote(b, ent.UserAgent)
	b = append(b, '\n')
	return b
}
