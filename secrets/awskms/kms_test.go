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
	"os"
	"testing"

	kmsv2 "github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
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
	useV2    bool
	client   *kms.KMS
	clientV2 *kmsv2.Client
	close    func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, driver.Keeper, error) {
	return &keeper{useV2: h.useV2, keyID: keyID1, client: h.client, clientV2: h.clientV2}, &keeper{useV2: h.useV2, keyID: keyID2, client: h.client, clientV2: h.clientV2}, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	sess, _, done, _ := setup.NewAWSSession(ctx, t, region)
	return &harness{
		useV2:  false,
		client: kms.New(sess),
		close:  done,
	}, nil
}

func newHarnessV2(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	cfg, _, done, _ := setup.NewAWSv2Config(ctx, t, region)
	return &harness{
		useV2:    true,
		clientV2: kmsv2.NewFromConfig(cfg),
		close:    done,
	}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{v2: false}})
}

func TestConformanceV2(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarnessV2, []drivertest.AsTest{verifyAs{v2: true}})
}

type verifyAs struct {
	v2 bool
}

func (v verifyAs) Name() string {
	return "verify As function"
}

func (v verifyAs) ErrorCheck(k *secrets.Keeper, err error) error {
	var code string
	if v.v2 {
		var e smithy.APIError
		if !k.ErrorAs(err, &e) {
			return errors.New("Keeper.ErrorAs failed")
		}
		code = e.ErrorCode()
	} else {
		var e awserr.Error
		if !k.ErrorAs(err, &e) {
			return errors.New("Keeper.ErrorAs failed")
		}
		code = e.Code()
	}
	if code != kms.ErrCodeInvalidCiphertextException {
		return fmt.Errorf("got %q, want %q", code, kms.ErrCodeInvalidCiphertextException)
	}
	return nil
}

// KMS-specific tests.

func TestNoSessionProvidedError(t *testing.T) {
	if _, err := Dial(nil); err == nil {
		t.Error("got nil, want no AWS session provided")
	}
}

func TestNoConnectionError(t *testing.T) {
	prevAccessKey := os.Getenv("AWS_ACCESS_KEY")
	prevSecretKey := os.Getenv("AWS_SECRET_KEY")
	prevRegion := os.Getenv("AWS_REGION")
	os.Setenv("AWS_ACCESS_KEY", "myaccesskey")
	os.Setenv("AWS_SECRET_KEY", "mysecretkey")
	os.Setenv("AWS_REGION", "us-east-1")
	defer func() {
		os.Setenv("AWS_ACCESS_KEY", prevAccessKey)
		os.Setenv("AWS_SECRET_KEY", prevSecretKey)
		os.Setenv("AWS_REGION", prevRegion)
	}()
	sess, err := session.NewSession()
	if err != nil {
		t.Fatal(err)
	}

	client, err := Dial(sess)
	if err != nil {
		t.Fatal(err)
	}
	keeper := OpenKeeper(client, keyID1, nil)
	defer keeper.Close()

	if _, err := keeper.Encrypt(context.Background(), []byte("test")); err == nil {
		t.Error("got nil, want UnrecognizedClientException")
	}
}

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
		// OK, using V1.
		{"awskms://alias/my-key?awssdk=v1", false},
		// OK, using V2.
		{"awskms://alias/my-key?awssdk=v2", false},
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
