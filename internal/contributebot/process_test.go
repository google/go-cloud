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
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/github"
)

func TestDefaultRepoConfig(t *testing.T) {
	cfg := defaultRepoConfig()
	if _, err := regexp.Compile(cfg.IssueTitlePattern); err != nil {
		t.Error("Issue title pattern:", err)
	}
	if _, err := regexp.Compile(cfg.PullRequestTitlePattern); err != nil {
		t.Error("Pull request title pattern:", err)
	}
}

func TestProcessIssueEvent(t *testing.T) {
	const (
		defaultTitle = "foo: bar"
	)

	tests := []struct {
		description string
		action      string
		title       string
		prevTitle   string
		labels      []string
		want        *issueEdits
	}{
		// Closed issue should not be checked other than "in progress" label
		{
			description: "close with invalid title -> no change",
			action:      "closed",
			title:       "foo",
			want:        &issueEdits{},
		},
		// Remove "in progress" label from closed issues.
		{
			description: "close with random label -> no change",
			action:      "closed",
			title:       defaultTitle,
			labels:      []string{"foo"},
			want:        &issueEdits{},
		},
		{
			description: "open with in progress label -> no change",
			action:      "opened",
			title:       defaultTitle,
			labels:      []string{"in progress"},
			want:        &issueEdits{},
		},
		{
			description: "close with in progress label -> remove it",
			action:      "closed",
			title:       defaultTitle,
			labels:      []string{"in progress"},
			want: &issueEdits{
				RemoveLabels: []string{"in progress"},
			},
		},
		// Check issue title looks like "foo: bar".
		{
			description: "open with invalid issue title -> add comment",
			action:      "opened",
			title:       "foo",
			want: &issueEdits{
				AddComments: []string{defaultRepoConfig().IssueTitleResponse},
			},
		},
		{
			description: "edit on invalid issue title but title didn't change -> no change",
			action:      "edited",
			title:       "foo",
			prevTitle:   "foo",
			want:        &issueEdits{},
		},
		{
			description: "edit to invalid issue title -> add comment",
			action:      "edited",
			title:       "prev",
			prevTitle:   "foo",
			want: &issueEdits{
				AddComments: []string{defaultRepoConfig().IssueTitleResponse},
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
				Title:  github.String(tc.title),
			}
			var chg *github.EditChange
			if tc.action == "edited" {
				chg = &github.EditChange{}
				if tc.prevTitle != "" {
					title := struct {
						From *string `json:"from,omitempty"`
					}{From: github.String(tc.prevTitle)}
					chg.Title = &title
				}
			}
			data := &issueData{
				Action: tc.action,
				Issue:  iss,
				Change: chg,
			}
			got := processIssueEvent(defaultRepoConfig(), data)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestProcessPullRequestEvent(t *testing.T) {
	const (
		mainRepoOwner = "google"
		mainRepoName  = "go-cloud"
		defaultTitle  = "foo: bar"
		defaultAuthor = "octocat"
	)

	tests := []struct {
		description string
		action      string
		state       string
		reviewers   []string
		title       string
		prevTitle   string
		headOwner   string
		want        *pullRequestEdits
	}{
		// Skip processing when the PR is closed.
		{
			description: "closed with invalid title -> no change",
			title:       defaultTitle,
			state:       "closed",
			headOwner:   defaultAuthor,
			want:        &pullRequestEdits{},
		},
		// If the pull request is from a branch of the main repo, close it.
		{
			description: "open with branch from fork -> no change",
			action:      "opened",
			title:       defaultTitle,
			headOwner:   defaultAuthor,
			want:        &pullRequestEdits{},
		},
		{
			description: "open with branch from main repo -> close",
			action:      "opened",
			headOwner:   mainRepoOwner,
			want: &pullRequestEdits{
				Close:       true,
				AddComments: []string{branchesInForkCloseResponse},
			},
		},
		// Assign to reviewers.
		{
			description: "open with no assignee and a reviewer -> assign",
			action:      "opened",
			title:       defaultTitle,
			reviewers:   []string{"foo"},
			headOwner:   defaultAuthor,
			want:        &pullRequestEdits{AssignTo: []string{"foo"}},
		},
		{
			description: "open with no assignee and multiple reviewers -> assign",
			action:      "opened",
			title:       defaultTitle,
			reviewers:   []string{"foo", "bar"},
			headOwner:   defaultAuthor,
			want:        &pullRequestEdits{AssignTo: []string{"foo", "bar"}},
		},
		{
			description: "closed with no assignee and a reviewer -> no change",
			action:      "edited",
			title:       defaultTitle,
			state:       "closed",
			reviewers:   []string{"foo"},
			headOwner:   defaultAuthor,
			want:        &pullRequestEdits{},
		},
		// Check title looks like "foo: bar".
		{
			description: "open with invalid title -> add comment",
			action:      "opened",
			title:       "foo",
			headOwner:   defaultAuthor,
			want: &pullRequestEdits{
				AddComments: []string{defaultRepoConfig().PullRequestTitleResponse},
			},
		},
		{
			description: "edit on invalid title but title didn't change -> no change",
			action:      "edited",
			title:       "foo",
			prevTitle:   "foo",
			headOwner:   defaultAuthor,
			want:        &pullRequestEdits{},
		},
		{
			description: "edit to invalid title -> add comment",
			action:      "edited",
			title:       "prev",
			prevTitle:   "foo",
			headOwner:   defaultAuthor,
			want: &pullRequestEdits{
				AddComments: []string{defaultRepoConfig().PullRequestTitleResponse},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			pr := &github.PullRequest{
				Base: &github.PullRequestBranch{
					Repo: &github.Repository{
						ID: github.Int64(1234),
						Owner: &github.User{
							Login: github.String(mainRepoOwner),
						},
						Name: github.String(mainRepoName),
					},
					Ref: github.String("master"),
				},
				Head: &github.PullRequestBranch{
					// Repo will be filled in below.
					Ref: github.String("feature"),
				},
				Title: github.String(tc.title),
				State: github.String(tc.state),
			}
			for _, reviewer := range tc.reviewers {
				pr.RequestedReviewers = append(pr.RequestedReviewers, &github.User{Login: github.String(reviewer)})
			}
			if tc.headOwner == mainRepoOwner {
				pr.Head.Repo = pr.Base.Repo
			} else {
				pr.Head.Repo = &github.Repository{
					ID: github.Int64(5678),
					Owner: &github.User{
						Login: github.String(tc.headOwner),
					},
					Name: github.String(mainRepoName),
				}
			}

			var chg *github.EditChange
			if tc.action == "edited" {
				chg = &github.EditChange{}
				if tc.prevTitle != "" {
					title := struct {
						From *string `json:"from,omitempty"`
					}{From: github.String(tc.prevTitle)}
					chg.Title = &title
				}
			}
			data := &pullRequestData{
				Action:      tc.action,
				OwnerLogin:  mainRepoOwner,
				Repo:        mainRepoName,
				PullRequest: pr,
				Change:      chg,
			}
			got := processPullRequestEvent(defaultRepoConfig(), data)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}
