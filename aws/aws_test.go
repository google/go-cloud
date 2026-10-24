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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
		{
			name:    "Role with invalid ARN",
			query:   url.Values{"role": {"not-a-valid-arn"}},
			wantErr: true,
		},
		{
			name:    "Role with empty ARN",
			query:   url.Values{"role": {""}},
			wantErr: true,
		},
		{
			name:       "Role with valid ARN and region",
			query:      url.Values{"role": {"arn:aws:iam::123456789012:role/MyRole"}, "region": {"us-west-2"}},
			wantRegion: "us-west-2",
		},
		{
			name:    "Role options without role",
			query:   url.Values{"role_session_name": {"s"}},
			wantErr: true,
		},
		{
			name:    "Web identity token file without role",
			query:   url.Values{"web_identity_token_file": {"/tmp/token"}},
			wantErr: true,
		},
		{
			name:    "Role with invalid duration",
			query:   url.Values{"role": {"arn:aws:iam::123456789012:role/MyRole"}, "role_duration": {"bad"}},
			wantErr: true,
		},
		{
			name:    "Role with external ID and web identity",
			query:   url.Values{"role": {"arn:aws:iam::123456789012:role/MyRole"}, "role_external_id": {"e"}, "web_identity_token_file": {"/tmp/token"}},
			wantErr: true,
		},
		{
			name:    "Role with anonymous",
			query:   url.Values{"role": {"arn:aws:iam::123456789012:role/MyRole"}, "anonymous": {"true"}},
			wantErr: true,
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

// fakeSTS returns credentials whose access key ID is the requested action.
type fakeSTS struct {
	*httptest.Server
	got url.Values
}

func newFakeSTS(t *testing.T) *fakeSTS {
	f := &fakeSTS{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing STS request: %v", err)
		}
		f.got = r.PostForm
		action := r.PostForm.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<%[1]sResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <%[1]sResult>
    <Credentials>
      <AccessKeyId>%[1]s</AccessKeyId>
      <SecretAccessKey>secret</SecretAccessKey>
      <SessionToken>token</SessionToken>
      <Expiration>2100-01-01T00:00:00Z</Expiration>
    </Credentials>
  </%[1]sResult>
  <ResponseMetadata><RequestId>id</RequestId></ResponseMetadata>
</%[1]sResponse>`, action)
	}))
	t.Cleanup(f.Close)
	return f
}

func writeTokenFile(t *testing.T) string {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("my-oidc-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestV2ConfigFromURLParamsAssumeRole(t *testing.T) {
	ctx := context.Background()
	sts := newFakeSTS(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "base-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))

	const roleARN = "arn:aws:iam::123456789012:role/my-role"
	tests := []struct {
		name       string
		query      url.Values
		wantAction string
		wantForm   url.Values
	}{
		{
			name: "AssumeRole",
			query: url.Values{
				"role":              {roleARN},
				"role_session_name": {"my-session"},
				"role_external_id":  {"my-external-id"},
				"role_duration":     {"30m"},
			},
			wantAction: "AssumeRole",
			wantForm: url.Values{
				"RoleArn":         {roleARN},
				"RoleSessionName": {"my-session"},
				"ExternalId":      {"my-external-id"},
				"DurationSeconds": {"1800"},
			},
		},
		{
			name: "AssumeRoleWithWebIdentity",
			query: url.Values{
				"role":                    {roleARN},
				"role_session_name":       {"my-session"},
				"role_duration":           {"1h"},
				"web_identity_token_file": {writeTokenFile(t)},
			},
			wantAction: "AssumeRoleWithWebIdentity",
			wantForm: url.Values{
				"RoleArn":          {roleARN},
				"RoleSessionName":  {"my-session"},
				"WebIdentityToken": {"my-oidc-token"},
				"DurationSeconds":  {"3600"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := url.Values{}
			for k, v := range test.query {
				q[k] = v
			}
			// The service endpoint must not be used for STS.
			q.Set("endpoint", "http://localhost:1")
			cfg, err := gcaws.V2ConfigFromURLParams(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			creds, err := cfg.Credentials.Retrieve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if creds.AccessKeyID != test.wantAction {
				t.Errorf("got access key ID %q, want %q", creds.AccessKeyID, test.wantAction)
			}
			if got := sts.got.Get("Action"); got != test.wantAction {
				t.Errorf("got STS action %q, want %q", got, test.wantAction)
			}
			for k := range test.wantForm {
				if got, want := sts.got.Get(k), test.wantForm.Get(k); got != want {
					t.Errorf("got STS %s %q, want %q", k, got, want)
				}
			}
		})
	}
}
