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

// TestHexEscapeInjective verifies that two different inputs never escape to
// the same output for a given shouldEscape function, and that every escaped
// string still unescapes back to its original input. Regression test: a
// caller-supplied string that already contains the literal text of an
// escape sequence (e.g. "__0x2f__") used to be indistinguishable, after
// escaping, from a different string whose escaped rune produced that same
// text (e.g. a "/" that shouldEscape flagged). Two callers choosing
// different keys could therefore end up addressing the same escaped
// storage key.
func TestHexEscapeInjective(t *testing.T) {
	// Mirrors the "escape a '/' right after '..'" rule used by
	// blob/fileblob, blob/gcsblob, blob/s3blob, and blob/azureblob.
	dotDotSlash := func(r []rune, i int) bool {
		return i > 1 && r[i] == '/' && r[i-1] == '.' && r[i-2] == '.'
	}

	pairs := [][2]string{
		{"a/../b", "a/..__0x2f__b"},
		{"x/../y", "x/..__0x2f__y"},
		{"../z", "..__0x2f__z"},
	}
	for _, p := range pairs {
		e0 := HexEscape(p[0], dotDotSlash)
		e1 := HexEscape(p[1], dotDotSlash)
		if e0 == e1 {
			t.Errorf("collision: HexEscape(%q) == HexEscape(%q) == %q", p[0], p[1], e0)
		}
		if got := HexUnescape(e0); got != p[0] {
			t.Errorf("HexUnescape(HexEscape(%q)) = %q, want %q", p[0], got, p[0])
		}
		if got := HexUnescape(e1); got != p[1] {
			t.Errorf("HexUnescape(HexEscape(%q)) = %q, want %q", p[1], got, p[1])
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
