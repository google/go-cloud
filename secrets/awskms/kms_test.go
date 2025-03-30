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

package awskms

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/secrets"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
)

const (
	keyID1 = "alias/test-secrets"
	keyID2 = "alias/test-secrets2"
	region = "us-east-2"
)

type harness struct {
	client *kms.Client
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	return &keeper{keyID: keyID1, client: h.client}, &keeper{keyID: keyID2, client: h.client}, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	t.Helper()

	cfg, _, done, _ := setup.NewAWSv2Config(ctx, t, region)
	return &harness{
		client: kms.NewFromConfig(cfg),
		close:  done,
	}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct {
}

func (v verifyAs) Name() string {
	return "verify As function"
}

func (v verifyAs) ErrorCheck(k *secrets.Keeper, err error) error {
	var e smithy.APIError
	if !k.ErrorAs(err, &e) {
		return errors.New("Keeper.ErrorAs failed")
	}
	code := e.ErrorCode()
	want := (&types.InvalidCiphertextException{}).ErrorCode()
	if code != want {
		return fmt.Errorf("got %q, want %q", code, want)
	}
	return nil
}

// KMS-specific tests.

func TestEncryptionContext(t *testing.T) {
	tests := []struct {
		Existing map[string]string
		URL      string
		WantErr  bool
		Want     map[string]string
	}{
		// None before or after.
		{nil, "http://foo", false, nil},
		// New parameter.
		{nil, "http://foo?context_foo=bar", false, map[string]string{"foo": "bar"}},
		// 2 new parameters.
		{nil, "http://foo?context_foo=bar&context_abc=baz", false, map[string]string{"foo": "bar", "abc": "baz"}},
		// Multiple values.
		{nil, "http://foo?context_foo=bar&context_foo=baz", true, nil},
		// Existing, no new.
		{map[string]string{"foo": "bar"}, "http://foo", false, map[string]string{"foo": "bar"}},
		// No-conflict merge.
		{map[string]string{"foo": "bar"}, "http://foo?context_abc=baz", false, map[string]string{"foo": "bar", "abc": "baz"}},
		// Overwrite merge.
		{map[string]string{"foo": "bar"}, "http://foo?context_foo=baz", false, map[string]string{"foo": "baz"}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("existing %v URL %v", test.Existing, test.URL), func(t *testing.T) {
			opts := KeeperOptions{
				EncryptionContext: test.Existing,
			}
			u, err := url.Parse(test.URL)
			if err != nil {
				t.Fatal(err)
			}
			err = addEncryptionContextFromURLParams(&opts, u.Query())
			if (err != nil) != test.WantErr {
				t.Fatalf("got err %v, want error? %v", err, test.WantErr)
			}
			if diff := cmp.Diff(opts.EncryptionContext, test.Want); diff != "" {
				t.Errorf("diff %v", diff)
			}
		})
	}
}

func TestOpenKeeper(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK, by alias.
		{"awskms://alias/my-key", false},
		// OK, by ARN with empty Host.
		{"awskms:///arn:aws:kms:us-east-1:932528106278:alias/gocloud-test", false},
		// OK, by ARN with empty Host.
		{"awskms:///arn:aws:kms:us-east-1:932528106278:key/8be0dcc5-da0a-4164-a99f-649015e344b5", false},
		// OK, overriding region.
		{"awskms://alias/my-key?region=us-west1", false},
		// OK, adding EncryptionContext.
		{"awskms://alias/my-key?context_abc=foo&context_def=bar", false},
		// Multiple values for an EncryptionContext.
		{"awskms://alias/my-key?context_abc=foo&context_abc=bar", true},
		// Unknown parameter.
		{"awskms://alias/my-key?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		keeper, err := secrets.OpenKeeper(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			if err = keeper.Close(); err != nil {
				t.Errorf("%s: got error during close: %v", test.URL, err)
			}
		}
	}
}
