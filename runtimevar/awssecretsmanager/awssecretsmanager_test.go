// Copyright 2020 The Go Cloud Development Kit Authors
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

package awssecretsmanager

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/googleapis/gax-go"
	"gocloud.dev/internal/retry"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

// This constant records the region used for the last --record.
// If you want to use --record mode,
// 1. Update this constant to your AWS region.
// TODO(issue #300): Use Terraform to get this.
const region = "us-east-2"

type harness struct {
	session client.ConfigProvider
	closer  func()
}

const maxClientRequestTokenLen = 64

// AWS Secrets Manager requires unique token for Create and Update requests to ensure idempotency.
// From the other side, request data must be deterministic in order to make tests reproducible.
// generateClientRequestToken generates token which is unique per test session but deterministic.
func generateClientRequestToken(name string, data []byte) string {
	h := sha1.New()
	_, _ = h.Write(data)

	token := fmt.Sprintf("%s-%x", name, h.Sum(nil))

	// Token must have length less than or equal to 64
	if len(token) > maxClientRequestTokenLen {
		token = token[:maxClientRequestTokenLen]
	}

	return token
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	sess, _, done, _ := setup.NewAWSSession(context.Background(), t, region)

	return &harness{
		session: sess,
		closer:  done,
	}, nil
}

func (h *harness) MakeWatcher(_ context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.session, name, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	svc := secretsmanager.New(h.session)
	awsName := aws.String(name)
	token := aws.String(generateClientRequestToken(name, val))

	// From AWS Secrets Manager docs:
	// An asynchronous background process performs the actual secret deletion, so there
	// can be a short delay before the operation completes. If you write code to
	// delete and then immediately recreate a secret with the same name, ensure
	// that your code includes appropriate back off and retry logic.
	var backoff gax.Backoff
	if *setup.Record {
		backoff.Initial = 5 * time.Second
		backoff.Max = 30 * time.Second
	} else {
		backoff.Max = time.Millisecond
	}

	return retry.Call(ctx, backoff,
		func(err error) bool {
			if awsErr, ok := err.(awserr.Error); ok {
				return awsErr.Code() == secretsmanager.ErrCodeInvalidRequestException
			}

			return false
		},
		func() error {
			_, err := svc.CreateSecretWithContext(ctx, &secretsmanager.CreateSecretInput{
				Name:               awsName,
				ClientRequestToken: token,
				SecretBinary:       val,
			})

			return err
		},
	)
}

var errNotSupported = errors.New("method not supported")

func (h *harness) UpdateVariable(_ context.Context, _ string, _ []byte) error {
	return errNotSupported
}

func (h *harness) DeleteVariable(_ context.Context, _ string) error {
	return errNotSupported
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return false }

func TestConformance(t *testing.T) {
	//drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var getParam *secretsmanager.GetSecretValueOutput
	if !s.As(&getParam) {
		return errors.New("Snapshot.As failed for GetSecretValueOutput")
	}
	var descParam *secretsmanager.DescribeSecretOutput
	if !s.As(&descParam) {
		return errors.New("Snapshot.As failed for DescribeSecretOutput")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var e awserr.Error
	if !v.ErrorAs(err, &e) {
		return errors.New("runtimevar.ErrorAs failed")
	}
	return nil
}

// Secrets Manager-specific tests.

func TestEquivalentError(t *testing.T) {
	tests := []struct {
		Err1, Err2 error
		Want       bool
	}{
		{Err1: errors.New("not aws"), Err2: errors.New("not aws"), Want: true},
		{Err1: errors.New("not aws"), Err2: errors.New("not aws but different")},
		{Err1: errors.New("not aws"), Err2: awserr.New("code1", "fail", nil)},
		{Err1: awserr.New("code1", "fail", nil), Err2: awserr.New("code2", "fail", nil)},
		{Err1: awserr.New("code1", "fail", nil), Err2: awserr.New("code1", "fail", nil), Want: true},
	}

	for _, test := range tests {
		got := equivalentError(test.Err1, test.Err2)
		if got != test.Want {
			t.Errorf("%v vs %v: got %v want %v", test.Err1, test.Err2, got, test.Want)
		}
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

	v, err := OpenVariable(sess, "variable-name", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	_, err = v.Watch(context.Background())
	if err == nil {
		t.Error("got nil want error")
	}
}

func TestOpenVariable(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"awssecretsmanager://myvar", false},
		// OK, setting region.
		{"awssecretsmanager://myvar?region=us-west-1", false},
		// OK, setting decoder.
		{"awssecretsmanager://myvar?decoder=string", false},
		// Invalid decoder.
		{"awssecretsmanager://myvar?decoder=notadecoder", true},
		// Invalid parameter.
		{"awssecretsmanager://myvar?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		v, err := runtimevar.OpenVariable(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
		if err == nil {
			v.Close()
		}
	}
}
