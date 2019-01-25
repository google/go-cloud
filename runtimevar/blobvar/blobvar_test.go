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

package blobvar

import (
	"context"
	"errors"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

type harness struct {
	bucket *blob.Bucket
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	return &harness{
		bucket: memblob.OpenBucket(nil),
	}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.bucket, name, decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	return h.bucket.WriteAll(ctx, name, val, nil)
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.bucket.WriteAll(ctx, name, val, nil)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	return h.bucket.Delete(ctx, name)
}

func (h *harness) Close() {}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	return nil
}

func (verifyAs) ErrorCheck(_ driver.Watcher, err error) error {
	var blobe error
	if !runtimevar.ErrorAs(err, &blobe) {
		return errors.New("runtimevar.ErrorAs failed")
	}
	if gcerrors.Code(err) == gcerrors.NotFound {
		return errors.New("expected error code to be other than NotFound to return false for wrapped error")
	}
	if gcerrors.Code(blobe) != gcerrors.NotFound {
		return errors.New("expected error code to be NotFound for unwrapped error")
	}
	return nil
}
