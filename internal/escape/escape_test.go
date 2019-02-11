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

func TestHexEscape(t *testing.T) {
	always := func([]rune, int) bool { return true }

	for _, tc := range []struct {
		description, s, want string
		should               func([]rune, int) bool
	}{
		{
			description: "empty string",
			s:           "",
			want:        "",
			should:      always,
		},
		{
			description: "first rune",
			s:           "hello world",
			want:        "__0x68__ello world",
			should:      func(_ []rune, i int) bool { return i == 0 },
		},
		{
			description: "last rune",
			s:           "hello world",
			want:        "hello worl__0x64__",
			should:      func(r []rune, i int) bool { return i == len(r)-1 },
		},
		{
			description: "runes in middle",
			s:           "hello  world",
			want:        "hello__0x20____0x20__world",
			should:      func(r []rune, i int) bool { return r[i] == ' ' },
		},
		{
			description: "unicode",
			s:           "☺☺",
			should:      always,
			want:        "__0x263a____0x263a__",
		},
	} {
		got := HexEscape(tc.s, tc.should)
		if got != tc.want {
			t.Errorf("%s: got escaped %q want %q", tc.description, got, tc.want)
		}
		got = HexUnescape(got)
		if got != tc.s {
			t.Errorf("%s: got unescaped %q want %q", tc.description, got, tc.s)
		}
	}
}

func TestHexEscapeUnescapeWeirdStrings(t *testing.T) {
	for name, s := range WeirdStrings {
		escaped := HexEscape(s, func(r []rune, i int) bool { return !IsASCIIAlphanumeric(r[i]) })
		unescaped := HexUnescape(escaped)
		if unescaped != s {
			t.Errorf("%s: got unescaped %q want %q", name, unescaped, s)
		}
	}
}

func TestHexUnescapeOnInvalid(t *testing.T) {
	// Unescaping of valid escape sequences is tested in TestEscape.
	// This only tests invalid escape sequences, so Unescape is expected
	// to do nothing.
	for _, s := range []string{
		"0x68",
		"_0x68_",
		"__0x68_",
		"_0x68__",
		"__1x68__",
		"__0y68__",
		"__0xag__",       // invalid hex digit
		"__0x8fffffff__", // out of int32 range
	} {
		got := HexUnescape(s)
		if got != s {
			t.Errorf("%s: got %q want %q", s, got, s)
		}
	}
}
