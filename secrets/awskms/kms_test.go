// Copyright 2019 The Go Cloud Authors
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
// limtations under the License.

package awskms

import (
	"context"
	"testing"

	"gocloud.dev/internal/testing/setup"

	"github.com/aws/aws-sdk-go/service/kms"
	"gocloud.dev/secrets/driver"
	"gocloud.dev/secrets/drivertest"
)

const (
	keyID  = "alias/test-secrets"
	region = "us-west-1"
)

type harness struct {
	client *kms.KMS
	close  func()
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Keeper, error) {
	return &keeper{
		keyID:  keyID,
		client: h.client,
	}, nil
}

func (h *harness) Close() {
	h.close()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	sess, _, done := setup.NewAWSSession(t, region)
	return &harness{
		client: kms.New(sess),
		close:  done,
	}, nil
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
