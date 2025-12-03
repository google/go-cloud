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
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	gcaws "gocloud.dev/aws"
)

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
		wantEndpoint *aws.Endpoint
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
			wantEndpoint: &aws.Endpoint{
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
		{
			name:  "Request checksum calculation when_supported",
			query: url.Values{"request_checksum_calculation": {"when_supported"}},
		},
		{
			name:  "Request checksum calculation when_required",
			query: url.Values{"request_checksum_calculation": {"when_required"}},
		},
		{
			name:  "Response checksum validation when_supported",
			query: url.Values{"response_checksum_validation": {"when_supported"}},
		},
		{
			name:  "Response checksum validation when_required",
			query: url.Values{"response_checksum_validation": {"when_required"}},
		},
		{
			name:  "Both checksum parameters",
			query: url.Values{"request_checksum_calculation": {"when_required"}, "response_checksum_validation": {"when_supported"}},
		},
		{
			name:    "Invalid request checksum value",
			query:   url.Values{"request_checksum_calculation": {"invalid"}},
			wantErr: true,
		},
		{
			name:    "Invalid response checksum value",
			query:   url.Values{"response_checksum_validation": {"invalid"}},
			wantErr: true,
		},
		{
			name:    "Empty request checksum value",
			query:   url.Values{"request_checksum_calculation": {""}},
			wantErr: true,
		},
		{
			name:    "Empty response checksum value",
			query:   url.Values{"response_checksum_validation": {""}},
			wantErr: true,
		},
		{
			name:  "Uppercase request checksum",
			query: url.Values{"request_checksum_calculation": {"WHEN_SUPPORTED"}},
		},
		{
			name:  "Mixed case response checksum",
			query: url.Values{"response_checksum_validation": {"When_Required"}},
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

			// Check checksum configuration based on query parameters
			if test.query.Has("request_checksum_calculation") {
				expectedValue := test.query.Get("request_checksum_calculation")
				var expectedChecksum aws.RequestChecksumCalculation
				switch strings.ToLower(expectedValue) {
				case "when_supported":
					expectedChecksum = aws.RequestChecksumCalculationWhenSupported
				case "when_required":
					expectedChecksum = aws.RequestChecksumCalculationWhenRequired
				}
				if got.RequestChecksumCalculation != expectedChecksum {
					t.Errorf("got RequestChecksumCalculation %v, want %v", got.RequestChecksumCalculation, expectedChecksum)
				}
			}

			if test.query.Has("response_checksum_validation") {
				expectedValue := test.query.Get("response_checksum_validation")
				var expectedChecksum aws.ResponseChecksumValidation
				switch strings.ToLower(expectedValue) {
				case "when_supported":
					expectedChecksum = aws.ResponseChecksumValidationWhenSupported
				case "when_required":
					expectedChecksum = aws.ResponseChecksumValidationWhenRequired
				}
				if got.ResponseChecksumValidation != expectedChecksum {
					t.Errorf("got ResponseChecksumValidation %v, want %v", got.ResponseChecksumValidation, expectedChecksum)
				}
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
			r, ok := got.Retryer().(*retry.Standard)
			if !ok {
				t.Errorf("expected a standard retryer, got %v, expected retry.Standard", r)
			}
		})
	}
}
