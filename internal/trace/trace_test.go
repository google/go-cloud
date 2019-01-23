// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package trace

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	octrace "go.opencensus.io/trace"
)

func TestToStatus(t *testing.T) {
	for _, testcase := range []struct {
		input error
		want  octrace.Status
	}{
		{
			errors.New("some random error"),
			octrace.Status{Code: -1, Message: "some random error"},
		},
	} {
		got := toStatus(testcase.input)
		if r := cmp.Diff(got, testcase.want); r != "" {
			t.Errorf("got -, want +:\n%s", r)
		}
	}
}
