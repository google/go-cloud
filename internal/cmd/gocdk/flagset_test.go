// Copyright 2019 The Go Cloud Development Kit Authors
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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/xerrors"
)

func TestFlagSet(t *testing.T) {
	t.Run("EmptyArgs", func(t *testing.T) {
		output := new(bytes.Buffer)
		set := newFlagSet(&processContext{stderr: output}, "foo")
		err := set.Parse([]string{})
		if err != nil {
			t.Errorf("set.Parse returned %v; want <nil>", err)
		}
		if output.Len() > 0 {
			t.Errorf("output = %q; want \"\"", output)
		}
	})
	t.Run("BadFlag", func(t *testing.T) {
		output := new(bytes.Buffer)
		set := newFlagSet(&processContext{stderr: output}, "foo")
		err := set.Parse([]string{"--bar"})
		if err == nil {
			t.Error("set.Parse returned nil; want error")
		} else if xerrors.Is(err, flag.ErrHelp) {
			t.Error("set.Parse returned flag.ErrHelp; want different error")
		}
		if output.Len() > 0 {
			t.Errorf("output = %q; want \"\"", output)
		}
	})
	t.Run("Help", func(t *testing.T) {
		output := new(bytes.Buffer)
		set := newFlagSet(&processContext{stderr: output}, "foo")
		err := set.Parse([]string{"--help"})
		if !xerrors.Is(err, flag.ErrHelp) {
			t.Errorf("set.Parse returned %v; want %v", err, flag.ErrHelp)
		}
		if output.Len() == 0 || strings.Contains(output.String(), flag.ErrHelp.Error()) {
			t.Errorf("output = %q; want non-empty and not to include %q", output, flag.ErrHelp.Error())
		}
	})
}

func TestUsagef(t *testing.T) {
	cause := xerrors.New("bork")
	tests := []struct {
		name string
		err  error

		wantMessage       string
		wantUnwrap        error
		wantFormatMessage string
		wantFormatError   error
	}{
		{
			name: "StringOnly",
			err:  usagef("hello %s", "world"),

			wantMessage:       "usage: hello world",
			wantUnwrap:        nil,
			wantFormatMessage: "usage: hello world",
			wantFormatError:   nil,
		},
		{
			name: "Wrap",
			err:  usagef("hello: %w", cause),

			wantMessage:       "usage: hello: bork",
			wantUnwrap:        cause,
			wantFormatMessage: "usage: hello",
			wantFormatError:   cause,
		},
		{
			name: "Opaque",
			err:  usagef("hello: %v", cause),

			wantMessage:       "usage: hello: bork",
			wantUnwrap:        nil,
			wantFormatMessage: "usage: hello",
			wantFormatError:   cause,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !xerrors.As(test.err, new(usageError)) {
				t.Errorf("usagef returned a non-usageError: %#v", test.err)
			}
			if got := test.err.Error(); got != test.wantMessage {
				t.Errorf("err.Error() = %q; want %q", got, test.wantMessage)
			}
			if got := xerrors.Unwrap(test.err); !xerrors.Is(got, test.wantUnwrap) {
				t.Errorf("xerrors.Unwrap(err) = %v; want <nil>", got)
			}

			formatMsg, next := formatError(test.err)
			if formatMsg != test.wantFormatMessage {
				t.Errorf("after err.FormatError(p), message = %q; want %q", formatMsg, test.wantFormatMessage)
			}
			if !xerrors.Is(next, test.wantFormatError) {
				t.Errorf("err.FormatError(p) = %v; want %v", next, test.wantFormatError)
			}
		})
	}
}

// Ensure that usageError implements fmt.Formatter for Go 1.12 and below.
var _ fmt.Formatter = usageError{}

func formatError(err error) (message string, next error) {
	f, ok := err.(xerrors.Formatter)
	if !ok {
		return err.Error(), nil
	}
	p := new(testErrorPrinter)
	next = f.FormatError(p)
	return strings.TrimSuffix(p.message.String(), "\n"), next
}

type testErrorPrinter struct {
	message  strings.Builder
	inDetail bool
}

func (p *testErrorPrinter) Print(args ...interface{}) {
	if p.inDetail {
		return
	}
	fmt.Fprintln(&p.message, args...)
}

func (p *testErrorPrinter) Printf(format string, args ...interface{}) {
	if p.inDetail {
		return
	}
	fmt.Fprintf(&p.message, format, args...)
	if s := p.message.String(); len(s) == 0 || s[len(s)-1] != '\n' {
		p.message.WriteByte('\n')
	}
}

func (p *testErrorPrinter) Detail() bool {
	p.inDetail = true
	return true
}
