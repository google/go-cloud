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

package octest

import (
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/oc"
)

// Call holds the expected contents of a measured call.
// It is used for both metric and trace comparison.
type Call struct {
	Method string
	Code   gcerrors.ErrorCode
}

func formatSpanData(s *trace.SpanData) string {
	if s == nil {
		return "missing"
	}
	return fmt.Sprintf("<Name: %q, Code: %d>", s.Name, s.Code)
}

func formatCall(c *Call) string {
	if c == nil {
		return "nothing"
	}
	return fmt.Sprintf("<Name: %q, Code: %d>", c.Method, c.Code)
}

// Diff compares the list of spans and metric counts obtained from OpenCensus
// instrumentation (using the TestExporter in this package, or similar) with an
// expected list of calls. Only the name and code are compared. Order matters for
// traces (though not for metrics).
func Diff(gotSpans []*trace.SpanData, gotRows []*view.Row, namePrefix, provider string, want []Call) string {
	ds := diffSpans(gotSpans, namePrefix, want)
	dc := diffCounts(gotRows, namePrefix, provider, want)
	if len(ds) > 0 {
		ds = "trace: " + ds + "\n"
	}
	if len(dc) > 0 {
		dc = "metrics: " + dc
	}
	return ds + dc
}

func diffSpans(got []*trace.SpanData, prefix string, want []Call) string {
	var diffs []string
	add := func(i int, g *trace.SpanData, w *Call) {
		diffs = append(diffs, fmt.Sprintf("#%d: got %s, want %s", i, formatSpanData(g), formatCall(w)))
	}

	for i := 0; i < len(got) || i < len(want); i++ {
		switch {
		case i >= len(got):
			add(i, nil, &want[i])
		case i >= len(want):
			add(i, got[i], nil)
		case got[i].Name != prefix+"."+want[i].Method || got[i].Code != int32(want[i].Code):
			w := want[i]
			w.Method = prefix + "." + w.Method
			add(i, got[i], &w)
		}
	}
	return strings.Join(diffs, "\n")
}

func diffCounts(got []*view.Row, prefix, provider string, wantCalls []Call) string {
	var want []*view.Row
	for _, wc := range wantCalls {
		want = append(want, &view.Row{
			Tags: []tag.Tag{
				{Key: oc.MethodKey, Value: prefix + "." + wc.Method},
				{Key: oc.ProviderKey, Value: provider},
				{Key: oc.StatusKey, Value: fmt.Sprint(wc.Code)},
			},
			Data: &view.CountData{Value: 1},
		})
	}
	return cmp.Diff(got, want, cmpopts.SortSlices(lessRow))
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
