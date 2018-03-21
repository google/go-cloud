// Copyright 2018 Google LLC
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

package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	baselog "log"
	"os"
	"strings"

	cloud "github.com/vsekhar/go-cloud"
)

const localServiceName = "local"

func init() {
	RegisterLogProvider(localServiceName, localProvider)
}

type localStructuredLogEntry struct {
	ctx context.Context
	l   Logger
	m   map[string]interface{}
}

func (e *localStructuredLogEntry) add(name string, x interface{}) StructuredLogEntry {
	e.m[name] = x
	return e
}

func (e *localStructuredLogEntry) toJSON() (string, error) {
	d, err := json.Marshal(e.m)
	if err != nil {
		return "", err
	}
	e.m = nil // panic on subsequent use
	return string(d), err
}

func (e *localStructuredLogEntry) String(name string, x string) StructuredLogEntry {
	return e.add(name, x)
}

func (e *localStructuredLogEntry) Int32(name string, x int32) StructuredLogEntry {
	return e.add(name, x)
}

func (e *localStructuredLogEntry) Send() {
	s, err := e.toJSON()
	if err != nil {
		e.l.Log(e.ctx, "failed to marshal structured log entry")
	}
	e.l.Log(e.ctx, s)
}

func (e *localStructuredLogEntry) Sync() error {
	s, err := e.toJSON()
	if err != nil {
		return err
	}
	e.l.LogSync(e.ctx, s)
	return nil
}

const callDepth = 4

type localLogger struct {
	l *baselog.Logger
}

func (l *localLogger) Log(_ context.Context, msg string) {
	l.l.Output(callDepth, msg)
}

func (l *localLogger) LogSync(_ context.Context, msg string) error {
	l.l.Output(callDepth, msg)
	return nil
}

func (l *localLogger) LogStructured(ctx context.Context) StructuredLogEntry {
	return &localStructuredLogEntry{
		ctx: ctx,
		l:   l,
		m:   make(map[string]interface{}),
	}
}

// URIs are of the format: local://prefix:path/to/file
func localProvider(_ context.Context, uri string) (Logger, error) {
	service, rest, err := cloud.Service(uri)
	if err != nil {
		return nil, fmt.Errorf("Bad local log uri: %s", uri)
	}
	if service != localServiceName {
		panic("bad provider routing: " + uri)
	}
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("Bad local log uri, should be 'local://prefix:path/to/logfile': %s", uri)
	}
	prefix := parts[0]
	path := parts[1]

	var w io.Writer
	switch strings.ToLower(path) {
	case "stdout":
		w = os.Stdout
	case "stderr":
		w = os.Stderr
	default:
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		w = f
	}
	return &localLogger{
		l: baselog.New(w, prefix, baselog.LstdFlags|baselog.Lshortfile),
	}, nil
}
