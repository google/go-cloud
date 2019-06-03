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

func TestImageRefForCloudRun(t *testing.T) {
	tests := []struct {
		name            string
		localImage      string
		launchSpecifier map[string]interface{}

		want    string
		wantErr bool
	}{
		{
			name:       "gcr.io passthrough",
			localImage: "gcr.io/example/foo",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
			},
			want: "gcr.io/example/foo",
		},
		{
			name:       "eu.gcr.io passthrough",
			localImage: "eu.gcr.io/example/foo",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
			},
			want: "eu.gcr.io/example/foo",
		},
		{
			name:       "prepend gcr.io",
			localImage: "foo",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
			},
			want: "gcr.io/spec-project/foo",
		},
		{
			name:       "prepend gcr.io with tag",
			localImage: "foo:bar",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
			},
			want: "gcr.io/spec-project/foo:bar",
		},
		{
			name:            "project ID not set",
			localImage:      "foo:bar",
			launchSpecifier: map[string]interface{}{},
			wantErr:         true,
		},
		{
			name:       "launch specifier image name latest",
			localImage: "foo",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
				"image_name": "eu.gcr.io/something-else/baz",
			},
			want: "eu.gcr.io/something-else/baz",
		},
		{
			name:       "launch specifier image name with local tag",
			localImage: "foo:bar",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
				"image_name": "eu.gcr.io/something-else/baz",
			},
			want: "eu.gcr.io/something-else/baz:bar",
		},
		{
			name:       "launch specifier image name with local registry",
			localImage: "example.com:8080/foo/bar",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
				"image_name": "eu.gcr.io/something-else/quux",
			},
			want: "eu.gcr.io/something-else/quux",
		},
		{
			name:       "launch specifier image name with local tag and registry",
			localImage: "example.com:8080/foo/bar:baz",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
				"image_name": "eu.gcr.io/something-else/quux",
			},
			want: "eu.gcr.io/something-else/quux:baz",
		},
		{
			name:       "launch specifier non-GCR image name",
			localImage: "foo",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
				"image_name": "foo",
			},
			wantErr: true,
		},
		{
			name:       "launch specifier image and local both contain gcr.io",
			localImage: "gcr.io/foo/bar:baz",
			launchSpecifier: map[string]interface{}{
				"project_id": "spec-project",
				"image_name": "eu.gcr.io/something-else/quux",
			},
			want: "eu.gcr.io/something-else/quux:baz",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := imageRefForCloudRun(test.localImage, test.launchSpecifier)
			if got != test.want || (err != nil) != test.wantErr {
				wantErr := "<nil>"
				if test.wantErr {
					wantErr = "<error>"
				}
				t.Errorf("imageRefForCloudRun(%q, %v) = %q, %v; want %q, %s", test.localImage, test.launchSpecifier, got, err, test.want, wantErr)
			}
		})
	}
}

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
		name, tag, digest := parseImageRef(test.s)
		if name != test.name || tag != test.tag || digest != test.digest {
			t.Errorf("parseImageRef(%q) = %q, %q, %q; want %q, %q, %q", test.s, name, tag, digest, test.name, test.tag, test.digest)
		}
	}
}
