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

	"github.com/google/go-cmp/cmp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/oc"
	"gocloud.dev/internal/testing/octest"
)

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(blob.OpenCensusViews)
	defer te.Unregister()

	bytes := []byte("foo")
	b := memblob.OpenBucket(nil)
	defer b.Close()
	if err := b.WriteAll(ctx, "key", bytes, nil); err != nil {
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

	const driver = "gocloud.dev/blob/memblob"

	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/blob", driver, []octest.Call{
		{Method: "NewWriter", Code: gcerrors.OK},
		{Method: "NewRangeReader", Code: gcerrors.OK},
		{Method: "Attributes", Code: gcerrors.OK},
		{Method: "Delete", Code: gcerrors.OK},
		{Method: "NewRangeReader", Code: gcerrors.NotFound},
	})
	if diff != "" {
		t.Error(diff)
	}

	// Find and verify the bytes read/written metrics.
	var sawRead, sawWritten bool
	tags := []tag.Tag{{Key: oc.ProviderKey, Value: driver}}
	for !sawRead || !sawWritten {
		data := <-te.Stats
		switch data.View.Name {
		case "gocloud.dev/blob/bytes_read":
			if sawRead {
				continue
			}
			sawRead = true
		case "gocloud.dev/blob/bytes_written":
			if sawWritten {
				continue
			}
			sawWritten = true
		default:
			continue
		}
		if diff := cmp.Diff(data.Rows[0].Tags, tags, cmp.AllowUnexported(tag.Key{})); diff != "" {
			t.Errorf("tags for %s: %s", data.View.Name, diff)
			continue
		}
		sd, ok := data.Rows[0].Data.(*view.SumData)
		if !ok {
			t.Errorf("%s: data is %T, want SumData", data.View.Name, data.Rows[0].Data)
			continue
		}
		if got := int(sd.Value); got < len(bytes) {
			t.Errorf("%s: got %d, want at least %d", data.View.Name, got, len(bytes))
		}
	}
}
