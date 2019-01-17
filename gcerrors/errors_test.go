// Copyright 2019 The Go Cloud Authors
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

package gcerrors

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestNewf(t *testing.T) {
	e := Newf(Internal, nil, "a %d b", 3)
	got := e.Error()
	want := "Internal: a 3 b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCode(t *testing.T) {
	for _, test := range []struct {
		in   error
		want ErrorCode
	}{
		{nil, OK},
		{New(AlreadyExists, nil, ""), AlreadyExists},
		{io.EOF, Unknown},
	} {
		got := Code(test.in)
		if got != test.want {
			t.Errorf("%v: got %s, want %s", test.in, got, test.want)
		}
	}
}

func TestFormatting(t *testing.T) {
	for i, test := range []struct {
		err  *Error
		verb string
		want []string // regexps, one per line
	}{
		{
			New(NotFound, nil, "message"),
			"%v",
			[]string{`^NotFound: message$`},
		},
		{
			New(NotFound, nil, "message"),
			"%+v",
			[]string{
				`^NotFound: message:$`,
				`\s+gocloud.dev/gcerrors.TestFormatting$`,
				`\s+.*/go-cloud/gcerrors/errors_test.go:\d+$`,
				`^\s*$`, // blank line at end
			},
		},
		{
			New(AlreadyExists, errors.New("wrapped"), "message"),
			"%v",
			[]string{`^AlreadyExists: message: wrapped$`},
		},
		{
			New(AlreadyExists, errors.New("wrapped"), "message"),
			"%+v",
			[]string{
				`^AlreadyExists: message:$`,
				`^\s+gocloud.dev/gcerrors.TestFormatting$`,
				`^\s+.*/go-cloud/gcerrors/errors_test.go:\d+$`,
				`^\s+- wrapped$`,
			},
		},
		{
			New(AlreadyExists, errors.New("wrapped"), ""),
			"%v",
			[]string{`^AlreadyExists: wrapped`},
		},
		{
			New(AlreadyExists, errors.New("wrapped"), ""),
			"%+v",
			[]string{
				`^AlreadyExists:`,
				`^\s+gocloud.dev/gcerrors.TestFormatting$`,
				`^\s+.*/go-cloud/gcerrors/errors_test.go:\d+$`,
				`^\s+- wrapped$`,
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			gotString := fmt.Sprintf(test.verb, test.err)
			gotLines := strings.Split(gotString, "\n")
			if got, want := len(gotLines), len(test.want); got != want {
				t.Fatalf("got %d lines, want %d. got:\n%s", got, want, gotString)
			}
			for j, gl := range gotLines {
				matched, err := regexp.MatchString(test.want[j], gl)
				if err != nil {
					t.Fatal(err)
				}
				if !matched {
					t.Fatalf("line #%d: got %q, which doesn't match %q", j, gl, test.want[j])
				}
			}
		})
	}
}
