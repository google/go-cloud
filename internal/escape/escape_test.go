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

package escape

import (
	"testing"
)

func TestEscape(t *testing.T) {
	always := func(_ string, _ int) bool { return true }

	for _, tc := range []struct {
		description, s, want string
		should               func(string, int) bool
	}{
		{
			description: "empty string",
			should:      always,
		},
		{
			description: "first byte",
			s:           "hello world",
			want:        "__0x68__ello world",
			should:      func(_ string, i int) bool { return i == 0 },
		},
		{
			description: "last byte",
			s:           "hello world",
			want:        "hello worl__0x64__",
			should:      func(s string, i int) bool { return i == len(s)-1 },
		},
		{
			description: "bytes in middle",
			s:           "hello  world",
			want:        "hello__0x20____0x20__world",
			should:      func(s string, i int) bool { return s[i] == ' ' },
		},
		{
			description: "unicode",
			s:           "☺☺",
			should:      always,
			want:        "__0xE2____0x98____0xBA____0xE2____0x98____0xBA__",
		},
	} {
		got := Escape(tc.should, tc.s)
		if got != tc.want {
			t.Errorf("%s: got escaped %q want %q", tc.description, got, tc.want)
		}
		got = Unescape(got)
		if got != tc.s {
			t.Errorf("%s: got unescaped %q want %q", tc.description, got, tc.s)
		}
	}
}

func TestUnescape(t *testing.T) {
	// Unescaping of valid escape sequences is tested in TestEscape.
	// This only tests invalid escape sequences, so Unescape is expected
	// to do nothing.
	for _, s := range []string{
		"",
		"0x68",
		"_0x68_",
		"__0x68_",
		"_0x68__",
		"__1x68__",
		"__0y68__",
		"__0xAG__",
		"__0xff__",
	} {
		got := Unescape(s)
		if got != s {
			t.Errorf("%s: got %q want %q", s, got, s)
		}
	}
}
