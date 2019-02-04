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

package gcerrors

import (
	"context"
	"io"
	"testing"

	"gocloud.dev/internal/gcerr"
)

type wrappedErr struct {
	err error
}

func (w wrappedErr) Error() string { return "wrapped" }

func (w wrappedErr) Unwrap() error { return w.err }

func TestCode(t *testing.T) {
	for _, test := range []struct {
		in   error
		want ErrorCode
	}{
		{nil, OK},
		{gcerr.New(AlreadyExists, nil, 1, ""), AlreadyExists},
		{wrappedErr{gcerr.New(PermissionDenied, nil, 1, "")}, PermissionDenied},
		{context.Canceled, Canceled},
		{context.DeadlineExceeded, DeadlineExceeded},
		{wrappedErr{context.Canceled}, Canceled},
		{wrappedErr{context.DeadlineExceeded}, DeadlineExceeded},
		{io.EOF, Unknown},
	} {
		got := Code(test.in)
		if got != test.want {
			t.Errorf("%v: got %s, want %s", test.in, got, test.want)
		}
	}
}
