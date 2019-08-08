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

package docstore_test

import (
	"context"
	"testing"

	"gocloud.dev/docstore"
	"gocloud.dev/docstore/memdocstore"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/octest"
)

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(docstore.OpenCensusViews)
	defer te.Unregister()

	coll, err := memdocstore.OpenCollection("_id", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()

	// ActionList.Do.
	if err := coll.Create(ctx, map[string]interface{}{"_id": "a", "count": 0}); err != nil {
		t.Fatal(err)
	}

	// Query.Get.
	iter := coll.Query().Get(ctx)
	iter.Stop()

	const driver = "gocloud.dev/docstore/memdocstore"

	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/docstore", driver, []octest.Call{
		{Method: "ActionList.Do", Code: gcerrors.OK},
		{Method: "Query.Get", Code: gcerrors.OK},
	})
	if diff != "" {
		t.Error(diff)
	}
}
