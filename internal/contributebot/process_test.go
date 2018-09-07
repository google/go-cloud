// Copyright 2018 The Go Cloud Authors
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

package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/github"
)

func TestProcessIssueEvent(t *testing.T) {
	tests := []struct {
		description string
		action      string
		labels      []string
		want        *issueEdits
	}{
		// Remove "in progress" label from closed issues.
		{
			description: "close with random label -> no change",
			action:      "closed",
			labels:      []string{"foo"},
			want:        &issueEdits{},
		},
		{
			description: "open with in progress label -> no change",
			action:      "opened",
			labels:      []string{"in progress"},
			want:        &issueEdits{},
		},
		{
			description: "close with in progress label -> remove it",
			action:      "closed",
			labels:      []string{"in progress"},
			want: &issueEdits{
				RemoveLabels: []string{"in progress"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			lbls := make([]github.Label, len(tc.labels))
			for i, label := range tc.labels {
				lbls[i] = github.Label{Name: &label}
			}
			iss := &github.Issue{
				Labels: lbls,
			}
			data := &issueData{
				Action: tc.action,
				Issue:  iss,
			}
			got := processIssueEvent(data)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}
