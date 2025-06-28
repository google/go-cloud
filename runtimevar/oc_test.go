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

package runtimevar_test

import (
	"context"
	"testing"

	"go.opencensus.io/stats/view"
	"gocloud.dev/internal/oc"
	"gocloud.dev/internal/testing/octest"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/constantvar"
)

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(runtimevar.OpenCensusViews)
	defer te.Unregister()

	v := constantvar.New(1)
	defer v.Close()
	if _, err := v.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = v.Watch(cctx)

	seen := false
	const driver = "gocloud.dev/runtimevar/constantvar"
	for _, row := range te.Counts() {
		if _, ok := row.Data.(*view.CountData); !ok {
			continue
		}
		if row.Tags[0].Key == oc.ProviderKey && row.Tags[0].Value == driver {
			seen = true
			break
		}
	}
	if !seen {
		t.Errorf("did not see count row with provider=%s", driver)
	}
}
