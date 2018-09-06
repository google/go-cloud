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

// Action represents an action to be taken.
type Action interface {
	// Description returns a human-readable description of what the action will do.
	Description() string
	// Do executes the action.
	Do(ctx context.Context, client *github.Client, owner, repo string, issueNumber int) error
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
func processIssueEvent(data *issueData) []Action {
	var actions []Action
	log.Printf("Identifying actions for issue: %v", data)

	if data.Action == "closed" && hasLabel(data.Issue, inProgressLabel) {
		actions = append(actions, &removeIssueLabel{label: inProgressLabel})
	}
	log.Printf("-> Identified %d action(s)", len(actions))
	return actions
}

// executeActions executes actions in order, aborting on error.
func executeActions(ctx context.Context, client *github.Client, owner, repo string, num int, actions []Action) error {
	for _, action := range actions {
		log.Printf("  Taking action: %s", action.Description())
		if err := action.Do(ctx, client, owner, repo, num); err != nil {
			log.Printf("    failed: %v", err)
			return err
		}
		log.Printf("    success!")
	}
	return nil
}

// removeIssueLabel removes a label from an issue.
type removeIssueLabel struct {
	label string
}

func (a *removeIssueLabel) Description() string {
	return fmt.Sprintf("remove %q label", a.label)
}

func (a *removeIssueLabel) Do(ctx context.Context, client *github.Client, owner, repo string, issueNumber int) error {
	_, err := client.Issues.RemoveLabelForIssue(ctx, owner, repo, issueNumber, a.label)
	return err
}
