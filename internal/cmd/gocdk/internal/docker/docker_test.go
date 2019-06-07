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

package docker

import (
	"testing"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		s      string
		name   string
		tag    string
		digest string
	}{
		{"", "", "", ""},
		{"foo", "foo", "", ""},
		{"foo:bar", "foo", ":bar", ""},
		{"foo:bar@sha256:xyzzy", "foo", ":bar", "@sha256:xyzzy"},
		{":foo", "", ":foo", ""},
		{"foo:", "foo", ":", ""},
		{"example.com:8080/foo/bar", "example.com:8080/foo/bar", "", ""},
		{"example.com:8080/foo/bar:baz", "example.com:8080/foo/bar", ":baz", ""},
		{"example:!foo", "example:!foo", "", ""},
	}
	for _, test := range tests {
		name, tag, digest := ParseImageRef(test.s)
		if name != test.name || tag != test.tag || digest != test.digest {
			t.Errorf("parseImageRef(%q) = %q, %q, %q; want %q, %q, %q", test.s, name, tag, digest, test.name, test.tag, test.digest)
		}
	}
}
