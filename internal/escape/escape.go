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

// Package escape includes helpers for escaping and unescaping strings.
package escape

// Escape returns s, with all bytes for which shouldEscape returns true
// escaped to "__0xXX__", where XX is the hex representation of the byte
// value.
//
// shouldEscape takes the whole string and an index instead of a single
// byte because some escape decisions require context. For example, we
// might want to escape the second "/" in "//" but not the first one.
func Escape(s string, shouldEscape func(s string, i int) bool) string {
	// Do a first pass to see how many bytes need escaping.
	var toEscape []int
	for i := 0; i < len(s); i++ {
		if shouldEscape(s, i) {
			toEscape = append(toEscape, i)
		}
	}
	if len(toEscape) == 0 {
		return s
	}
	// Each escaped byte turns into 8 bytes ("__0xXX__"),
	// so there's an extra 7 bytes for each.
	escaped := make([]byte, len(s)+7*len(toEscape))
	n := 0 // current index into toEscape
	j := 0 // current index into escaped
	for i := 0; i < len(s); i++ {
		if n < len(toEscape) && i == toEscape[n] {
			b := s[i]
			escaped[j] = '_'
			escaped[j+1] = '_'
			escaped[j+2] = '0'
			escaped[j+3] = 'x'
			escaped[j+4] = "0123456789ABCDEF"[b>>4]
			escaped[j+5] = "0123456789ABCDEF"[b&15]
			escaped[j+6] = '_'
			escaped[j+7] = '_'
			j += 8
			n++
		} else {
			escaped[j] = s[i]
			j++
		}
	}
	return string(escaped)
}

// Unescape reverses Escape.
func Unescape(s string) string {
	var unescaped []byte
	for i := 0; i < len(s); i++ {
		if i+7 < len(s) &&
			s[i] == '_' &&
			s[i+1] == '_' &&
			s[i+2] == '0' &&
			s[i+3] == 'x' &&
			ishex(s[i+4]) &&
			ishex(s[i+5]) &&
			s[i+6] == '_' &&
			s[i+7] == '_' {
			// Unescape this byte.
			if unescaped == nil {
				// This is the first byte we've encountered that
				// needed unescaping. Allocate a buffer and copy any
				// previous bytes.
				unescaped = make([]byte, 0, len(s))
				if i != 0 {
					unescaped = append(unescaped, []byte(s[0:i])...)
				}
			}
			unescaped = append(unescaped, unhex(s[i+4])<<4|unhex(s[i+5]))
			// We used an extra 7 bytes from s, so move forward.
			i += 7
		} else if unescaped != nil {
			unescaped = append(unescaped, s[i])
		}
	}
	if unescaped == nil {
		return s
	}
	return string(unescaped)
}

// ishex returns true if c is a valid part of a hexadecimal number.
func ishex(b byte) bool {
	switch {
	case '0' <= b && b <= '9':
		return true
	case 'A' <= b && b <= 'F':
		return true
	}
	return false
}

// unhex returns the hexadecimal value of the hexadecimal byte b.
// For example, unhex('A') returns 10.
func unhex(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10
	}
	return 0
}
