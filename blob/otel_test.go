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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/oteltest"
	"testing"
)

func TestOpenTelemetry(t *testing.T) {
	ctx := t.Context()

	te := oteltest.NewTestExporter(t, blob.OpenTelemetryViews)
	defer func() { _ = te.Shutdown(ctx) }()

	bytes := []byte("hello world")
	b := memblob.OpenBucket(nil)
	defer func() { _ = b.Close() }()

	if err := b.WriteAll(ctx, "key", bytes, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadAll(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Attributes(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.ListPage(ctx, blob.FirstPageToken, 3, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadAll(ctx, "noSuchKey"); err == nil {
		t.Fatal("got nil, want error")
	}

	const driver = "gocloud.dev/blob/memblob"

	spans := te.GetSpans()
	metrics := te.GetMetrics(ctx)

	diff := oteltest.Diff(spans.Snapshots(), metrics, "gocloud.dev/blob", driver, []oteltest.Call{
		{Method: "NewWriter", Code: gcerrors.OK},
		{Method: "NewRangeReader", Code: gcerrors.OK},
		{Method: "Attributes", Code: gcerrors.OK},
		{Method: "ListPage", Code: gcerrors.OK},
		{Method: "Delete", Code: gcerrors.OK},
		{Method: "NewRangeReader", Code: gcerrors.NotFound},
	})
	if diff != "" {
		t.Error(diff)
	}

	// Find and verify the bytes read/written metrics.
	var sawRead, sawWritten bool
	providerAttr := attribute.Key("gocdk_provider")

	for _, sm := range metrics {

		for _, data := range sm.Metrics {

			switch data.Name {
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

			providerVal, ok := sm.Scope.Attributes.Value(providerAttr)
			if !ok || providerVal.AsString() != driver {
				t.Errorf("provider tags for %s is : %s instead of :%s", data.Name, providerVal.AsString(), driver)
				continue
			}
			sd, ok := data.Data.(metricdata.Sum[int64])
			if !ok {
				t.Errorf("%s: data is %T, want SumData", data.Name, data.Data)
				continue
			}
			if got := int(sd.DataPoints[0].Value); got < len(bytes) {
				t.Errorf("%s: got %d, want at least %d", data.Name, got, len(bytes))
			}

			if sawRead && sawWritten {
				return
			}
		}
	}
}
