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

	"go.opencensus.io/trace"
	"gocloud.dev/gcerrors"
)

// Span holds the expected contents of a span obtained from tracing.
type Span struct {
	Name string
	Code gcerrors.ErrorCode
}

func formatSpanData(s *trace.SpanData) string {
	if s == nil {
		return "missing"
	}
	return fmt.Sprintf("<Name: %q, Code: %d>", s.Name, s.Code)
}

func formatSpan(s *Span) string {
	if s == nil {
		return "nothing"
	}
	return fmt.Sprintf("<Name: %q, Code: %d>", s.Name, s.Code)
}

// DiffSpans compares the list of spans obtained from OpenCensus tracing
// (using the TestExporter in this package, or similar) with an expected
// list of spans. Only the name and code of the spans are compared.
// Order matters.
func DiffSpans(got []*trace.SpanData, want []Span) string {
	var diffs []string
	add := func(i int, g *trace.SpanData, w *Span) {
		diffs = append(diffs, fmt.Sprintf("#%d: got %s, want %s", i, formatSpanData(g), formatSpan(w)))
	}

	for i := 0; i < len(got) || i < len(want); i++ {
		switch {
		case i >= len(got):
			add(i, nil, &want[i])
		case i >= len(want):
			add(i, got[i], nil)
		case got[i].Name != want[i].Name || got[i].Code != int32(want[i].Code):
			add(i, got[i], &want[i])
		}
	}
	if len(diffs) == 0 {
		return ""
	}
	return "\n" + strings.Join(diffs, "\n")
}
