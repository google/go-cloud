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
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/go-github/github"
)

const (
	inProgressLabel = "in progress"
)

// issueData is information about an issue event.
// See the github documentation for more details about the fields:
// https://godoc.org/github.com/google/go-github/github#IssuesEvent
type issueData struct {
	// Action that this event is for.
	// Possible values are: "assigned", "unassigned", "labeled", "unlabeled", "opened", "closed", "reopened", "edited".
	Action string
	// Issue the event is for.
	Issue *github.Issue
	// Change made as part of the event.
	Change *github.EditChange
}

func (i *issueData) String() string {
	return fmt.Sprintf("[%s issue #%d]", i.Action, i.Issue.GetNumber())
}

// hasLabel returns true iff the issue has the given label.
func hasLabel(iss *github.Issue, label string) bool {
	for i := range iss.Labels {
		if iss.Labels[i].GetName() == label {
			return true
		}
	}
	return false
}

// processIssueEvent identifies actions that should be taken based on the issue
// event represented by data.
// Returned actions will be executed in order, aborting on error.
func processIssueEvent(data *issueData) *issueEdits {
	edits := &issueEdits{}
	log.Printf("Identifying actions for issue: %v", data)

	if data.Action == "closed" && hasLabel(data.Issue, inProgressLabel) {
		edits.RemoveLabels = append(edits.RemoveLabels, inProgressLabel)
	}
	log.Printf("-> %v", edits)
	return edits
}

// issueEdits capture all of the edits to be made to an issue.
type issueEdits struct {
	RemoveLabels []string
}

func (i *issueEdits) String() string {
	var actions []string
	for _, label := range i.RemoveLabels {
		actions = append(actions, fmt.Sprintf("removing %q label", label))
	}
	if len(actions) == 0 {
		return "[no changes]"
	}
	return strings.Join(actions, ", ")
}

// Execute applies all of the requested edits, aborting on error.
func (i *issueEdits) Execute(ctx context.Context, client *github.Client, owner, repo string, num int) error {
	for _, label := range i.RemoveLabels {
		_, err := client.Issues.RemoveLabelForIssue(ctx, owner, repo, num, label)
		if err != nil {
			return err
		}
	}
	return nil
}
