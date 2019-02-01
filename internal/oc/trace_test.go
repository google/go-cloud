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

package oc

import (
	"regexp"
	"testing"
)

type testDriver struct{}

func TestProviderName(t *testing.T) {
	for _, test := range []struct {
		in   interface{}
		want string
	}{
		{nil, ""},
		{testDriver{}, "gocloud.dev/internal/oc"},
		{&testDriver{}, "gocloud.dev/internal/oc"},
		{regexp.Regexp{}, "regexp"},
	} {
		got := ProviderName(test.in)
		if got != test.want {
			t.Errorf("%v: got %q, want %q", test.in, got, test.want)
		}
	}
}
