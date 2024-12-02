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
	"context"
	"net/url"
	"reflect"
	"testing"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsv2retry "github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/go-cmp/cmp"
	gcaws "gocloud.dev/aws"
)

func TestConfigFromURLParams(t *testing.T) {
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
			name:    "disable_ssl true",
			query:   url.Values{"disable_ssl": {"true"}},
			wantCfg: &aws.Config{DisableSSL: aws.Bool(true)},
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
			name:    "s3_force_path_style true",
			query:   url.Values{"s3_force_path_style": {"true"}},
			wantCfg: &aws.Config{S3ForcePathStyle: aws.Bool(true)},
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

func TestUseV2(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
		want  bool
	}{
		{
			name:  "No overrides",
			query: url.Values{},
			want:  true,
		},
		{
			name:  "unused param",
			query: url.Values{"foo": {"bar"}},
			want:  true,
		},
		{
			name:  "force v1",
			query: url.Values{"awssdk": {"v1"}},
			want:  false,
		},
		{
			name:  "force v1 cap",
			query: url.Values{"awssdk": {"V1"}},
		},
		{
			name:  "force v2",
			query: url.Values{"awssdk": {"v2"}},
			want:  true,
		},
		{
			name:  "force v2 cap",
			query: url.Values{"awssdk": {"V2"}},
			want:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := gcaws.UseV2(test.query)
			if test.want != got {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestV2ConfigFromURLParams(t *testing.T) {
	const service = "s3"
	const region = "us-east-1"
	const partitionID = "aws"
	ctx := context.Background()
	tests := []struct {
		name         string
		query        url.Values
		wantRegion   string
		wantErr      bool
		wantEndpoint *awsv2.Endpoint
	}{
		{
			name:  "No overrides",
			query: url.Values{},
		},
		{
			name:    "Invalid query parameter",
			query:   url.Values{"foo": {"bar"}},
			wantErr: true,
		},
		{
			name:       "Region",
			query:      url.Values{"region": {"my_region"}},
			wantRegion: "my_region",
		},
		{
			name:  "Endpoint and hostname immutable",
			query: url.Values{"endpoint": {"foo"}, "hostname_immutable": {"true"}},
			wantEndpoint: &awsv2.Endpoint{
				PartitionID:       partitionID,
				SigningRegion:     region,
				URL:               "foo",
				HostnameImmutable: true,
			},
		},
		{
			name:  "FIPS and dual stack",
			query: url.Values{"fips": {"true"}, "dualstack": {"true"}},
		},
		{
			name:  "anonymous",
			query: url.Values{"anonymous": {"true"}},
		},
		{
			name:  "Rate limit capacity",
			query: url.Values{"rate_limiter_capacity": {"500"}},
		},
		// Can't test "profile", since AWS validates that the profile exists.
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := gcaws.V2ConfigFromURLParams(ctx, test.query)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
				return
			}
			if err != nil {
				return
			}
			if test.wantRegion != "" && got.Region != test.wantRegion {
				t.Errorf("got region %q, want %q", got.Region, test.wantRegion)
			}

			if test.wantEndpoint != nil {
				if got.EndpointResolverWithOptions == nil {
					t.Fatalf("expected an EndpointResolverWithOptions, got nil")
				}
				gotE, err := got.EndpointResolverWithOptions.ResolveEndpoint(service, region)
				if err != nil {
					return
				}
				if !reflect.DeepEqual(gotE, *test.wantEndpoint) {
					t.Errorf("got endpoint %+v, want %+v", gotE, *test.wantEndpoint)
				}
			}

			// Unfortunately, we can't look at the options set for the rate limiter.
			r, ok := got.Retryer().(*awsv2retry.Standard)
			if !ok {
				t.Errorf("expected a standard retryer, got %v, expected awsv2retry.Standard", r)
			}
		})
	}
}
