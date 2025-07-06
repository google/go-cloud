// Copyright 2019-2025 The Go Cloud Development Kit Authors
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
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/memdocstore"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/oteltest"
	"testing"
)

func TestOpenTelemetry(t *testing.T) {
	ctx := context.Background()

	// Setup the test exporter for both trace and metrics.
	te := oteltest.NewTestExporter(t, docstore.OpenTelemetryViews)
	defer te.Shutdown(ctx)

	// Open a collection for testing.
	coll, err := memdocstore.OpenCollection("_id", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer coll.Close()

	// Test ActionList.Do by creating a document.
	if err := coll.Create(ctx, map[string]interface{}{"_id": "a", "count": 0}); err != nil {
		t.Fatal(err)
	}

	// Test Query.Get.
	iter := coll.Query().Get(ctx)
	iter.Stop()

	spanStubs := te.GetSpans()
	metrics := te.GetMetrics(ctx)
	const (
		pkgName = "gocloud.dev/docstore"
		driver  = "gocloud.dev/docstore/memdocstore"
	)

	diff := oteltest.Diff(spanStubs.Snapshots(), metrics, pkgName, driver, []oteltest.Call{
		{Method: "ActionList.Do", Code: gcerrors.OK},
		{Method: "Query.Get", Code: gcerrors.OK},
	})
	if diff != "" {
		t.Error(diff)
	}
}
