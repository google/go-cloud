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
	"os"
	"path"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

type harness struct {
	dir    string
	bucket *blob.Bucket
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	dir := path.Join(os.TempDir(), "go-cloud-blobvar")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil, err
	}
	b, err := fileblob.OpenBucket(dir, nil)
	if err != nil {
		return nil, err
	}
	return &harness{dir: dir, bucket: b}, nil
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

func (h *harness) Close() {
	h.bucket.Close()
	_ = os.RemoveAll(h.dir)
}

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

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var perr *os.PathError
	if !v.ErrorAs(err, &perr) {
		return errors.New("runtimevar.ErrorAs failed with *os.PathError")
	}
	return nil
}
