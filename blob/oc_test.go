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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/internal/oc"
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
	// TODO(jba): after #1213, use DiffSpans.

	// Wait for counts. Expect all counts to arrive in the same view.Data.
	for {
		data := <-te.Stats
		if _, ok := data.Rows[0].Data.(*view.CountData); !ok {
			continue
		}
		// There should be a count of 1 for each of these sets of tags.
		// Rows may appear in any order, but tags within a row will be sorted
		// by key name (see https://godoc.org/go.opencensus.io/stats/view#Row.Equal).
		var want []*view.Row
		for _, m := range []string{"Attributes", "Delete", "NewRangeReader", "NewWriter"} {
			want = append(want, &view.Row{
				Tags: []tag.Tag{
					{Key: oc.MethodKey, Value: "gocloud.dev/blob." + m},
					{Key: oc.ProviderKey, Value: "gocloud.dev/blob/memblob"},
					{Key: oc.StatusKey, Value: "OK"},
				},
				Data: &view.CountData{Value: 1},
			})
		}
		want = append(want, &view.Row{
			Tags: []tag.Tag{
				{Key: oc.MethodKey, Value: "gocloud.dev/blob.NewRangeReader"},
				{Key: oc.ProviderKey, Value: "gocloud.dev/blob/memblob"},
				{Key: oc.StatusKey, Value: "NotFound"},
			},
			Data: &view.CountData{Value: 1},
		})
		diff := cmp.Diff(data.Rows, want, cmpopts.SortSlices(lessRow))
		if diff != "" {
			t.Error(diff)
		}
		break
	}
}

// The order doesn't matter; we just need a deterministic ordering of rows.
func lessRow(r1, r2 *view.Row) bool { return rowKey(r1) < rowKey(r2) }

func rowKey(r *view.Row) string {
	var vals []string
	for _, t := range r.Tags {
		vals = append(vals, t.Value)
	}
	return strings.Join(vals, "|")
}
