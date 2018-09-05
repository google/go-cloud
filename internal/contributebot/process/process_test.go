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

package process

import (
	"reflect"
	"testing"

	"github.com/google/go-github/github"
)

func TestIssue(t *testing.T) {
	tests := []struct {
		description string
		action      string
		labels      []string
		want        []string
	}{
		{
			description: "close with random label -> no change",
			action:      "closed",
			labels:      []string{"foo"},
		},
		{
			description: "open with in progress label -> no change",
			action:      "opened",
			labels:      []string{"in progress"},
		},
		{
			description: "close with in progress label -> remove it",
			action:      "closed",
			labels:      []string{"in progress"},
			want:        []string{"remove \"in progress\" label"},
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
			data := &IssueData{
				Action: tc.action,
				Issue:  iss,
			}
			actions := Issue(data)
			var got []string
			if len(actions) > 0 {
				got = make([]string, len(actions))
				for i, action := range actions {
					got[i] = action.Description()
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got\n  %v\nwant\n  %v", got, tc.want)
			}
		})
	}
}
