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
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	secretsmanagerv2 "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/smithy-go"
	"github.com/googleapis/gax-go/v2"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
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
	useV2    bool
	session  client.ConfigProvider
	clientV2 *secretsmanagerv2.Client
	closer   func()
}

// waitForMutation uses check to wait until a mutation has taken effect.
// The check function should return nil to indicate success (the mutation has
// taken effect), an error with gcerrors.ErrorCode == NotFound to trigger
// a retry, or any other error to signal permanent failure.
func waitForMutation(ctx context.Context, check func() error) error {
	backoff := gax.Backoff{Multiplier: 1.0}
	var initial time.Duration
	if *setup.Record {
		// When recording, wait 3 seconds and then poll every 2s.
		initial = 3 * time.Second
		backoff.Initial = 2 * time.Second
	} else {
		// During replay, we don't wait at all.
		// The recorded file may have retries, but we don't need to actually wait between them.
		backoff.Initial = 1 * time.Millisecond
	}
	backoff.Max = backoff.Initial

	// Sleep before the check, since we know it doesn't take effect right away.
	time.Sleep(initial)

	// retryIfNotFound returns true if err is NotFound.
	var retryIfNotFound = func(err error) bool { return gcerrors.Code(err) == gcerrors.NotFound }

	// Poll until the mtuation is seen.
	return retry.Call(ctx, backoff, retryIfNotFound, check)
}

// AWS Secrets Manager requires unique token for Create and Update requests to ensure idempotency.
// From the other side, request data must be deterministic in order to make tests reproducible.
// generateClientRequestToken generates token which is unique per test session but deterministic.
func generateClientRequestToken(name string, data []byte) string {
	const maxClientRequestTokenLen = 64
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
		useV2:   false,
		session: sess,
		closer:  done,
	}, nil
}

func newHarnessV2(t *testing.T) (drivertest.Harness, error) {
	cfg, _, done, _ := setup.NewAWSv2Config(context.Background(), t, region)
	return &harness{
		useV2:    true,
		clientV2: secretsmanagerv2.NewFromConfig(cfg),
		closer:   done,
	}, nil
}

func (h *harness) MakeWatcher(_ context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.useV2, h.session, h.clientV2, name, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	var err error
	var svc *secretsmanager.SecretsManager
	if h.useV2 {
		_, err = h.clientV2.CreateSecret(ctx, &secretsmanagerv2.CreateSecretInput{
			Name:               awsv2.String(name),
			ClientRequestToken: awsv2.String(generateClientRequestToken(name, val)),
			SecretBinary:       val,
		})
	} else {
		svc = secretsmanager.New(h.session)
		_, err = svc.CreateSecretWithContext(ctx, &secretsmanager.CreateSecretInput{
			Name:               aws.String(name),
			ClientRequestToken: aws.String(generateClientRequestToken(name, val)),
			SecretBinary:       val,
		})
	}
	if err != nil {
		return err
	}
	// Secret Manager is only eventually consistent, so we retry until we've
	// verified that the mutation was applied. This is still not a guarantee
	// but in practice seems to work well enough to make tests repeatable.
	return waitForMutation(ctx, func() error {
		var err error
		if h.useV2 {
			_, err = h.clientV2.GetSecretValue(ctx, &secretsmanagerv2.GetSecretValueInput{SecretId: awsv2.String(name)})
		} else {
			_, err = svc.GetSecretValueWithContext(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
		}
		if err == nil {
			// Create was seen.
			return nil
		}
		// Failure; we'll retry if it's a NotFound.
		w := &watcher{}
		return gcerr.New(w.ErrorCode(err), err, 1, "runtimevar")
	})
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	var svc *secretsmanager.SecretsManager
	var err error
	if h.useV2 {
		_, err = h.clientV2.PutSecretValue(ctx, &secretsmanagerv2.PutSecretValueInput{
			ClientRequestToken: awsv2.String(generateClientRequestToken(name, val)),
			SecretBinary:       val,
			SecretId:           awsv2.String(name),
		})
	} else {
		svc = secretsmanager.New(h.session)
		_, err = svc.PutSecretValueWithContext(ctx, &secretsmanager.PutSecretValueInput{
			ClientRequestToken: aws.String(generateClientRequestToken(name, val)),
			SecretBinary:       val,
			SecretId:           aws.String(name),
		})
	}
	if err != nil {
		return err
	}
	// Secret Manager is only eventually consistent, so we retry until we've
	// verified that the mutation was applied. This is still not a guarantee
	// but in practice seems to work well enough to make tests repeatable.
	return waitForMutation(ctx, func() error {
		var err error
		var bb []byte
		if h.useV2 {
			var getResp *secretsmanagerv2.GetSecretValueOutput
			getResp, err = h.clientV2.GetSecretValue(ctx, &secretsmanagerv2.GetSecretValueInput{SecretId: awsv2.String(name)})
			if err == nil {
				bb = getResp.SecretBinary
			}
		} else {
			var getResp *secretsmanager.GetSecretValueOutput
			getResp, err = svc.GetSecretValueWithContext(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
			if err == nil {
				bb = getResp.SecretBinary
			}
		}
		if err != nil {
			// Failure; we'll retry if it's a NotFound, but that's not
			// really expected for an Update.
			w := &watcher{}
			return gcerr.New(w.ErrorCode(err), err, 1, "runtimevar")
		}
		if !bytes.Equal(bb, val) {
			// Value hasn't been updated yet, return a NotFound to
			// trigger retry.
			return gcerr.Newf(gcerr.NotFound, nil, "updated value not seen yet")
		}
		// Update was seen.
		return nil
	})
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	var svc *secretsmanager.SecretsManager
	var err error
	if h.useV2 {
		_, err = h.clientV2.DeleteSecret(ctx, &secretsmanagerv2.DeleteSecretInput{
			ForceDeleteWithoutRecovery: aws.Bool(true),
			SecretId:                   awsv2.String(name),
		})
	} else {
		svc = secretsmanager.New(h.session)
		_, err = svc.DeleteSecretWithContext(ctx, &secretsmanager.DeleteSecretInput{
			ForceDeleteWithoutRecovery: aws.Bool(true),
			SecretId:                   aws.String(name),
		})
	}
	if err != nil {
		return err
	}
	// Secret Manager is only eventually consistent, so we retry until we've
	// verified that the mutation was applied. This is still not a guarantee
	// but in practice seems to work well enough to make tests repeatable.
	// Note that "success" after a delete is a NotFound error, so we massage
	// the err returned from DescribeSecret to reflect that.
	return waitForMutation(ctx, func() error {
		var err error
		if h.useV2 {
			_, err = h.clientV2.DescribeSecret(ctx, &secretsmanagerv2.DescribeSecretInput{SecretId: awsv2.String(name)})
		} else {
			_, err = svc.DescribeSecretWithContext(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
		}
		if err == nil {
			// Secret still exists, return a NotFound to trigger a retry.
			return gcerr.Newf(gcerr.NotFound, nil, "delete not seen yet")
		}
		w := &watcher{useV2: h.useV2}
		if w.ErrorCode(err) == gcerrors.NotFound {
			// Delete was seen.
			return nil
		}
		// Other errors are not retryable.
		return gcerr.New(w.ErrorCode(err), err, 1, "runtimevar")
	})
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{useV2: false}})
}

func TestConformanceV2(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarnessV2, []drivertest.AsTest{verifyAs{useV2: true}})
}

type verifyAs struct {
	useV2 bool
}

func (verifyAs) Name() string {
	return "verify As"
}

func (v verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	if v.useV2 {
		var getParam *secretsmanagerv2.GetSecretValueOutput
		if !s.As(&getParam) {
			return errors.New("Snapshot.As failed for GetSecretValueOutput")
		}
		var descParam *secretsmanagerv2.DescribeSecretOutput
		if !s.As(&descParam) {
			return errors.New("Snapshot.As failed for DescribeSecretOutput")
		}
		return nil
	}
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

func (va verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	if va.useV2 {
		var e smithy.APIError
		if !v.ErrorAs(err, &e) {
			return errors.New("Keeper.ErrorAs failed")
		}
		return nil
	}
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
		// OK, setting wait.
		{"awssecretsmanager://myvar?wait=5m", false},
		// Invalid wait.
		{"awssecretsmanager://myvar?wait=xx", true},
		// Invalid parameter.
		{"awssecretsmanager://myvar?param=value", true},
		// OK, using SDK V2.
		{"awssecretsmanager://myvar?decoder=string&awssdk=v2", false},
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
