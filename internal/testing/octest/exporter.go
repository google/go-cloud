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

// Package octest supports testing of OpenCensus integrations.
package octest

// This code was copied from cloud.google.com/go/internal/testutil/trace.go

import (
	"log"
	"sync"
	"time"

	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

// TestExporter is an exporter of OpenCensus traces and metrics, for testing.
// It should be created with NewTestExporter.
type TestExporter struct {
	mu    sync.Mutex
	spans []*trace.SpanData
	Stats chan *view.Data
}

// NewTestExporter creates a TestExporter and registers it with OpenCensus.
func NewTestExporter(views []*view.View) *TestExporter {
	te := &TestExporter{Stats: make(chan *view.Data)}

	// Register for metrics.
	view.RegisterExporter(te)
	// The reporting period will affect how long it takes to get stats (view.Data).
	// We want it short so tests don't take too long, but long enough so that all
	// the actions in a test complete.
	//   If the period is too short, then some actions may not be finished when the first
	// call to ExportView happens. diffCounts checks for matching counts, so it will
	// fail in that case.
	//   Tests that use the exporter (search for TestOpenCensus) are designed to avoid
	// network traffic or computation, so they finish quickly. But we must account for
	// the race detector, which slows everything down.
	view.SetReportingPeriod(100 * time.Millisecond)
	if err := view.Register(views...); err != nil {
		log.Fatal(err)
	}

	// Register for traces.
	trace.RegisterExporter(te)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	return te
}

// ExportSpan "exports" a span by remembering it.
func (te *TestExporter) ExportSpan(s *trace.SpanData) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.spans = append(te.spans, s)
}

// ExportView exports a view by writing it to the Stats channel.
func (te *TestExporter) ExportView(vd *view.Data) {
	if len(vd.Rows) > 0 {
		select {
		case te.Stats <- vd:
		default:
		}
	}
}

func (te *TestExporter) Spans() []*trace.SpanData {
	te.mu.Lock()
	defer te.mu.Unlock()
	return te.spans
}

// Counts returns the first exported data that includes aggregated counts.
func (te *TestExporter) Counts() []*view.Row {
	// Wait for counts. Expect all counts to arrive in the same view.Data.
	for {
		data := <-te.Stats
		if _, ok := data.Rows[0].Data.(*view.CountData); !ok {
			continue
		}
		return data.Rows
	}
}

// Unregister unregisters the exporter from OpenCensus.
func (te *TestExporter) Unregister() {
	view.UnregisterExporter(te)
	trace.UnregisterExporter(te)
	view.SetReportingPeriod(0) // reset to default value
}
