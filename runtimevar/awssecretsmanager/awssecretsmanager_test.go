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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
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
	client *secretsmanager.Client
	closer func()
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
	retryIfNotFound := func(err error) bool { return gcerrors.Code(err) == gcerrors.NotFound }

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
	t.Helper()

	cfg, _, done, _ := setup.NewAWSv2Config(context.Background(), t, region)
	return &harness{
		client: secretsmanager.NewFromConfig(cfg),
		closer: done,
	}, nil
}

func (h *harness) MakeWatcher(_ context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, name, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	if _, err := h.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:               aws.String(name),
		ClientRequestToken: aws.String(generateClientRequestToken(name, val)),
		SecretBinary:       val,
	}); err != nil {
		return err
	}
	// Secret Manager is only eventually consistent, so we retry until we've
	// verified that the mutation was applied. This is still not a guarantee
	// but in practice seems to work well enough to make tests repeatable.
	return waitForMutation(ctx, func() error {
		_, err := h.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
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
	if _, err := h.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		ClientRequestToken: aws.String(generateClientRequestToken(name, val)),
		SecretBinary:       val,
		SecretId:           aws.String(name),
	}); err != nil {
		return err
	}
	// Secret Manager is only eventually consistent, so we retry until we've
	// verified that the mutation was applied. This is still not a guarantee
	// but in practice seems to work well enough to make tests repeatable.
	return waitForMutation(ctx, func() error {
		var err error
		var bb []byte
		var getResp *secretsmanager.GetSecretValueOutput
		getResp, err = h.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
		if err == nil {
			bb = getResp.SecretBinary
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
	if _, err := h.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		ForceDeleteWithoutRecovery: aws.Bool(true),
		SecretId:                   aws.String(name),
	}); err != nil {
		return err
	}
	// Secret Manager is only eventually consistent, so we retry until we've
	// verified that the mutation was applied. This is still not a guarantee
	// but in practice seems to work well enough to make tests repeatable.
	// Note that "success" after a delete is a NotFound error, so we massage
	// the err returned from DescribeSecret to reflect that.
	return waitForMutation(ctx, func() error {
		_, err := h.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
		if err == nil {
			// Secret still exists, return a NotFound to trigger a retry.
			return gcerr.Newf(gcerr.NotFound, nil, "delete not seen yet")
		}
		w := &watcher{}
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
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct {
}

func (verifyAs) Name() string {
	return "verify As"
}

func (v verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
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
	var e smithy.APIError
	if !v.ErrorAs(err, &e) {
		return errors.New("Keeper.ErrorAs failed")
	}
	return nil
}

// Secrets Manager-specific tests.

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
