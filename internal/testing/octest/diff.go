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

	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"gocloud.dev/gcerrors"
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
	// Because OpenCensus keeps global state, running tests with -count N can result
	// in aggregate counts greater than 1. Also, other tests can contribute measurements.
	// So all we can do is make sure that each call appears with count at least 1.
	var diffs []string
	gotTags := map[string]bool{} // map of canonicalized row tags
	for _, row := range got {
		if _, ok := row.Data.(*view.CountData); !ok {
			diffs = append(diffs, fmt.Sprintf("row.Data is %T, want CountData", row.Data))
			continue
		}
		var tags []string
		for _, t := range row.Tags {
			tags = append(tags, t.Key.Name()+":"+t.Value)
		}
		gotTags[strings.Join(tags, ",")] = true
	}
	for _, wc := range wantCalls {
		mapKey := fmt.Sprintf("gocdk_method:%s.%s,gocdk_provider:%s,gocdk_status:%s",
			prefix, wc.Method, provider, fmt.Sprint(wc.Code))
		if !gotTags[mapKey] {
			diffs = append(diffs, fmt.Sprintf("missing %q", mapKey))
		}
	}
	return strings.Join(diffs, "\n")
}
