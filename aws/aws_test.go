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

package aws_test

import (
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/go-cmp/cmp"
	gcaws "gocloud.dev/aws"
)

func TestURLOpenerForParams(t *testing.T) {
	tests := []struct {
		name    string
		query   url.Values
		wantCfg *aws.Config
		wantErr bool
	}{
		{
			name:    "No overrides",
			query:   url.Values{},
			wantCfg: &aws.Config{},
		},
		{
			name:    "Invalid query parameter",
			query:   url.Values{"foo": {"bar"}},
			wantErr: true,
		},
		{
			name:    "Region",
			query:   url.Values{"region": {"my_region"}},
			wantCfg: &aws.Config{Region: aws.String("my_region")},
		},
		{
			name:    "Endpoint",
			query:   url.Values{"endpoint": {"foo"}},
			wantCfg: &aws.Config{Endpoint: aws.String("foo")},
		},
		{
			name:    "DisableSSL true",
			query:   url.Values{"disableSSL": {"true"}},
			wantCfg: &aws.Config{DisableSSL: aws.Bool(true)},
		},
		{
			name:    "DisableSSL false",
			query:   url.Values{"disableSSL": {"false"}},
			wantCfg: &aws.Config{DisableSSL: aws.Bool(false)},
		},
		{
			name:    "DisableSSL false",
			query:   url.Values{"disableSSL": {"invalid"}},
			wantErr: true,
		},
		{
			name:    "S3ForcePathStyle true",
			query:   url.Values{"s3ForcePathStyle": {"true"}},
			wantCfg: &aws.Config{S3ForcePathStyle: aws.Bool(true)},
		},
		{
			name:    "S3ForcePathStyle false",
			query:   url.Values{"s3ForcePathStyle": {"false"}},
			wantCfg: &aws.Config{S3ForcePathStyle: aws.Bool(false)},
		},
		{
			name:    "S3ForcePathStyle false",
			query:   url.Values{"s3ForcePathStyle": {"invalid"}},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := gcaws.ConfigFromURLParams(test.query)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(got, test.wantCfg); diff != "" {
				t.Errorf("opener.forParams(...) diff (-want +got):\n%s", diff)
			}
		})
	}
}
