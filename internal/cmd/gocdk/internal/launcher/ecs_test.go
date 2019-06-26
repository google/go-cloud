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

package launcher

import (
	"testing"
)

func TestImageRefForECS(t *testing.T) {
	tests := []struct {
		name            string
		localImage      string
		launchSpecifier map[string]interface{}

		want    string
		wantErr bool
	}{
		{
			name:       "ECR passthrough",
			localImage: "12345678.dkr.ecr.us-east-1.amazonaws.com/foo",
			want:       "12345678.dkr.ecr.us-east-1.amazonaws.com/foo",
		},
		{
			name:            "image name not set",
			localImage:      "foo:bar",
			launchSpecifier: map[string]interface{}{},
			wantErr:         true,
		},
		{
			name:       "launch specifier image name latest",
			localImage: "foo",
			launchSpecifier: map[string]interface{}{
				"image_name": "12345678.dkr.ecr.us-east-1.amazonaws.com/baz",
			},
			want: "12345678.dkr.ecr.us-east-1.amazonaws.com/baz",
		},
		{
			name:       "launch specifier image name with local tag",
			localImage: "foo:bar",
			launchSpecifier: map[string]interface{}{
				"image_name": "12345678.dkr.ecr.us-east-1.amazonaws.com/baz",
			},
			want: "12345678.dkr.ecr.us-east-1.amazonaws.com/baz:bar",
		},
		{
			name:       "launch specifier image name with local registry",
			localImage: "example.com:8080/foo/bar",
			launchSpecifier: map[string]interface{}{
				"image_name": "12345678.dkr.ecr.us-east-1.amazonaws.com/quux",
			},
			want: "12345678.dkr.ecr.us-east-1.amazonaws.com/quux",
		},
		{
			name:       "launch specifier image name with local tag and registry",
			localImage: "example.com:8080/foo/bar:baz",
			launchSpecifier: map[string]interface{}{
				"image_name": "12345678.dkr.ecr.us-east-1.amazonaws.com/quux",
			},
			want: "12345678.dkr.ecr.us-east-1.amazonaws.com/quux:baz",
		},
		{
			name:       "launch specifier non-ECR image name",
			localImage: "foo",
			launchSpecifier: map[string]interface{}{
				"image_name": "quux",
			},
			wantErr: true,
		},
		{
			name:       "launch specifier image and local both ECR",
			localImage: "901234.dkr.ecr.us-west-1.amazonaws.com/foo:bar",
			launchSpecifier: map[string]interface{}{
				"image_name": "12345678.dkr.ecr.us-east-1.amazonaws.com/quux",
			},
			want: "12345678.dkr.ecr.us-east-1.amazonaws.com/quux:bar",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := imageRefForECS(test.localImage, test.launchSpecifier)
			if got != test.want || (err != nil) != test.wantErr {
				wantErr := "<nil>"
				if test.wantErr {
					wantErr = "<error>"
				}
				t.Errorf("imageRefForECS(%q, %v) = %q, %v; want %q, %s", test.localImage, test.launchSpecifier, got, err, test.want, wantErr)
			}
		})
	}
}
