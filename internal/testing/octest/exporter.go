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
	"time"

	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

// TestExporter is an exporter of OpenCensus traces and metrics, for testing.
// It should be created with NewTestExporter.
type TestExporter struct {
	Spans []*trace.SpanData
	Stats chan *view.Data
}

// NewTestExporter creates a TestExporter and registers it with OpenCensus.
func NewTestExporter(views []*view.View) *TestExporter {
	te := &TestExporter{Stats: make(chan *view.Data)}

	// Register for metrics.
	view.RegisterExporter(te)
	view.SetReportingPeriod(time.Millisecond)
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
	te.Spans = append(te.Spans, s)
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

// Unregister unregisters the exporter from OpenCensus.
func (te *TestExporter) Unregister() {
	view.UnregisterExporter(te)
	trace.UnregisterExporter(te)
	view.SetReportingPeriod(0) // reset to default value
}
