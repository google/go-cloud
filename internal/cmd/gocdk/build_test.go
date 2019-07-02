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
	"testing"
)

func TestParseImageNameFromDockerfile(t *testing.T) {
	tests := []struct {
		name       string
		dockerfile string
		want       string
		wantErr    bool
	}{
		{
			name:       "Empty",
			dockerfile: "",
			wantErr:    true,
		},
		{
			name:       "NoComment",
			dockerfile: "FROM scratch\n",
			wantErr:    true,
		},
		{
			name: "CommentAtStart",
			dockerfile: "# gocdk-image: hello-world\n" +
				"FROM scratch\n",
			want: "hello-world",
		},
		{
			name: "CommentNoEOL",
			dockerfile: "FROM scratch\n" +
				"# gocdk-image: hello-world",
			want: "hello-world",
		},
		{
			name: "CommentLastLine",
			dockerfile: "FROM scratch\n" +
				"# gocdk-image: hello-world\n",
			want: "hello-world",
		},
		{
			name: "ImageContainsTag",
			dockerfile: "# gocdk-image: hello-world:bar\n" +
				"FROM scratch\n",
			wantErr: true,
		},
		{
			name: "ImageCustomRegistry",
			dockerfile: "# gocdk-image: example.com:8080/hello-world\n" +
				"FROM scratch\n",
			want: "example.com:8080/hello-world",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseImageNameFromDockerfile([]byte(test.dockerfile))
			if err != nil {
				if !test.wantErr {
					t.Errorf("unexpected error: %+v", err)
				}
				return
			}
			if got != test.want {
				t.Errorf("parseImageNameFromDockerfile(%q) = %q; want %q", test.dockerfile, got, test.want)
			}
		})
	}
}

func TestGenerateTag(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tag, err := generateTag()
		if err != nil {
			t.Fatal(err)
		}
		if !isValidDockerTag(tag) {
			t.Errorf("generateTag() = %q, <nil>; want valid Docker tag", tag)
			continue
		}
		if seen[tag] {
			t.Errorf("generateTag() = %q, <nil>; already seen", tag)
		}
		seen[tag] = true
	}
}

func isValidDockerTag(tag string) bool {
	// Following https://godoc.org/github.com/docker/distribution/reference
	if len(tag) == 0 || len(tag) > 128 || !isWordChar(tag[0]) {
		return false
	}
	for i := 1; i < len(tag); i++ {
		if !isWordChar(tag[i]) && tag[i] != '.' && tag[i] != '-' {
			return false
		}
	}
	return true
}

func isWordChar(b byte) bool {
	return '0' <= b && b <= '9' ||
		'A' <= b && b <= 'Z' ||
		'a' <= b && b <= 'z' ||
		b == '_'
}
