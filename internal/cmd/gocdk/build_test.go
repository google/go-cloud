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
