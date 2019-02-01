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

package blob_test

import (
	"context"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/octest"
)

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(blob.OpenCensusViews)
	defer te.Unregister()

	b := memblob.OpenBucket(nil)
	if err := b.WriteAll(ctx, "key", []byte("foo"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadAll(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Attributes(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadAll(ctx, "noSuchKey"); err == nil {
		t.Fatal("got nil, want error")
	}

	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/blob", "gocloud.dev/blob/memblob", []octest.Call{
		{"NewWriter", gcerrors.OK},
		{"NewRangeReader", gcerrors.OK},
		{"Attributes", gcerrors.OK},
		{"Delete", gcerrors.OK},
		{"NewRangeReader", gcerrors.NotFound},
	})
	if diff != "" {
		t.Error(diff)
	}
}
