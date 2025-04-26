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

package runtimevar_test

import (
	"context"
	"gocloud.dev/internal/testing/oteltest"
	"gocloud.dev/runtimevar/constantvar"
	"testing"
	"time"
)

func TestOpenTelemetry(t *testing.T) {
	ctx := context.Background()
	te := oteltest.NewTestExporter(t)
	defer te.Shutdown(ctx)

	v := constantvar.New(1)
	defer v.Close()
	if _, err := v.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = v.Watch(cctx)

	// Check for spans
	const driver = "gocloud.dev/runtimevar/constantvar"

	time.Sleep(2 * time.Second)
	// Check metrics - during migration, we may need to look for different metric names
	metrics := te.Metrics(ctx)
	metricsFound := false
	const metricName = "gocloud.dev/runtimevar/value_changes"

	for _, scopeMetric := range metrics {

		for _, attr := range scopeMetric.Scope.Attributes.ToSlice() {

			if attr.Value.AsString() == driver {
				for _, metric := range scopeMetric.Metrics {
					if metric.Name == metricName {
						metricsFound = true
						break
					}
				}
				if metricsFound {
					break
				}
			}
			if metricsFound {
				break
			}
		}
	}

	if !metricsFound {
		t.Errorf("did not see any expected metrics for runtimevar")
	}
}
