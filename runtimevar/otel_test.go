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
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/testing/oteltest"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/constantvar"
	"testing"
)

const (
	pkgName = "gocloud.dev/runtimevar"
	driver  = "gocloud.dev/runtimevar/constantvar"
)

func TestOpenTelemetry(t *testing.T) {
	ctx := context.Background()
	te := oteltest.NewTestExporter(t, runtimevar.OpenTelemetryViews)
	defer te.Shutdown(ctx)

	v := constantvar.New(1)
	defer v.Close()
	if _, err := v.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = v.Watch(cctx)

	// Check metrics - during migration, we may need to look for different metric names.
	metrics := te.GetMetrics(ctx)

	diff := oteltest.DiffMetrics(metrics, pkgName, driver, []oteltest.Call{
		{Method: "", Code: gcerrors.OK},
	})
	if diff != "" {
		t.Error(diff)
	}
}
