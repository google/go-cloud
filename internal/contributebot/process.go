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
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v18/github"
)

const (
	inProgressLabel             = "in progress"
	issueTitleComment           = "Please edit the title of this issue with the name of the affected package, or \"all\", followed by a colon, followed by a short summary of the issue. Example: `blob/gcsblob: not blobby enough`."
	pullRequestTitleComment     = "Please edit the title of this pull request with the name of the affected package, or \"all\", followed by a colon, followed by a short summary of the change. Example: `blob/gcsblob: improve comments`."
	branchesInForkCloseComment  = "Please create pull requests from your own fork instead of from branches in the main repository. Also, please delete this branch."
	missingLicenseHeaderComment = "Missing license header"
)

var (
	issueTitleRegexp           = regexp.MustCompile(`^[a-z0-9./-]+: .*$`)
	pullRequestTitleRegexp     = issueTitleRegexp
	checkLicenseFilenameRegexp = regexp.MustCompile(`^([a-z0-9.-]+).(go|tf|sh|sql|yaml)$`)
	generatedFileHeaderRegexp  = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
	licenseHeaderRegexp        = regexp.MustCompile(`Copyright ([0-9]+) The Go Cloud Authors$`)
)

// issueData is information about an issue event.
// See the github documentation for more details about the fields:
// https://godoc.org/github.com/google/go-github/github#IssuesEvent
type issueData struct {
	// Action that this event is for.
	// Possible values are: "assigned", "unassigned", "labeled", "unlabeled", "opened", "closed", "reopened", "edited".
	Action string
	// Repo is the repository the pull request wants to commit to.
	Repo string
	// Owner is the owner of the repository.
	Owner string
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

// titleChanged returns true iff the title changed.
func titleChanged(title string, edit *github.EditChange) bool {
	return edit != nil && edit.Title != nil && edit.Title.From != nil && *edit.Title.From != title
}

// processIssueEvent identifies actions that should be taken based on the issue
// event represented by data.
func processIssueEvent(data *issueData) *issueEdits {
	edits := &issueEdits{}
	log.Printf("Identifying actions for issue: %v", data)
	defer log.Printf("-> %v", edits)

	if data.Action == "closed" {
		if hasLabel(data.Issue, inProgressLabel) {
			edits.RemoveLabels = append(edits.RemoveLabels, inProgressLabel)
		}
		return edits
	}

	// Add a comment if the title doesn't match our regexp, and it's a new issue,
	// or an issue whose title has just been modified.
	if !issueTitleRegexp.MatchString(data.Issue.GetTitle()) &&
		(data.Action == "opened" || (data.Action == "edited" && titleChanged(data.Issue.GetTitle(), data.Change))) {
		edits.AddComments = append(edits.AddComments, issueTitleComment)
	}

	return edits
}

// issueEdits captures all of the edits to be made to an issue.
type issueEdits struct {
	RemoveLabels []string
	AddComments  []string
}

func (i *issueEdits) String() string {
	var actions []string
	for _, label := range i.RemoveLabels {
		actions = append(actions, fmt.Sprintf("removing %q label", label))
	}
	for _, comment := range i.AddComments {
		actions = append(actions, fmt.Sprintf("adding comment %q", comment))
	}
	if len(actions) == 0 {
		return "[no changes]"
	}
	return strings.Join(actions, ", ")
}

// Execute applies all of the requested edits, aborting on error.
func (i *issueEdits) Execute(ctx context.Context, client *github.Client, data *issueData) error {
	for _, label := range i.RemoveLabels {
		_, err := client.Issues.RemoveLabelForIssue(ctx, data.Owner, data.Repo, data.Issue.GetNumber(), label)
		if err != nil {
			return err
		}
	}
	for _, comment := range i.AddComments {
		_, _, err := client.Issues.CreateComment(ctx, data.Owner, data.Repo, data.Issue.GetNumber(), &github.IssueComment{
			Body: github.String(comment)})
		if err != nil {
			return err
		}
	}
	return nil
}

// pullRequestData is information about a pull request event.
// See the github documentation for more details about the fields:
// https://developer.github.com/v3/activity/events/types/#pullrequestevent
type pullRequestData struct {
	// Action that this event is for.
	// Possible values are: "assigned", "unassigned", "labeled", "unlabeled",
	// "opened", "closed", "reopened", "edited".
	Action string
	// OwnerLogin is the owner's name of the repository.
	OwnerLogin string
	// Repo is the name of the repository the pull request wants to commit to.
	Repo string
	// PullRequest the event is for.
	PullRequest *github.PullRequest
	// Change made as part of the event.
	Change *github.EditChange
}

func (pr *pullRequestData) String() string {
	return fmt.Sprintf("[%s #%d]", pr.Action, pr.PullRequest.GetNumber())
}

// processPullRequestEvent identifies actions that should be taken based on the
// pull request event represented by data.
func processPullRequestEvent(data *pullRequestData) *pullRequestEdits {
	edits := &pullRequestEdits{}
	log.Printf("Identifying actions for pull request: %v", data)
	defer log.Printf("-> %v", edits)
	pr := data.PullRequest

	// Skip the process when the PR is closed, we check this here instead of when
	// calling processPullRequest so that it is easier to add any process in future
	// for closed PR's.
	if pr.GetState() == "closed" {
		return edits
	}

	// If the pull request is not from a fork, close it and request that it comes
	// from a fork instead.
	if data.Action == "opened" && !pr.GetHead().GetRepo().GetFork() {
		edits.Close = true
		edits.AddComments = append(edits.AddComments, branchesInForkCloseComment)
		// Short circuit since we're closing anyway.
		return edits
	}

	// If unassigned, assign to the first requested reviewer.
	if pr.GetAssignee() == nil && len(pr.RequestedReviewers) > 0 {
		for _, r := range pr.RequestedReviewers {
			edits.AssignTo = append(edits.AssignTo, r.GetLogin())
		}
	}

	// Add a comment if the title doesn't match our regexp, and it's a new issue,
	// or an issue whose title has just been modified.
	if !pullRequestTitleRegexp.MatchString(pr.GetTitle()) &&
		(data.Action == "opened" || (data.Action == "edited" && titleChanged(pr.GetTitle(), data.Change))) {
		edits.AddComments = append(edits.AddComments, pullRequestTitleComment)
	}

	return edits
}

// pullRequestEdits captures all of the edits to be made to an issue.
type pullRequestEdits struct {
	Close       bool
	AssignTo    []string
	AddComments []string
}

func (i *pullRequestEdits) String() string {
	var actions []string
	if i.Close {
		actions = append(actions, "close")
	}
	if len(i.AssignTo) > 0 {
		actions = append(actions, fmt.Sprintf("assign to %s", strings.Join(i.AssignTo, " + ")))
	}
	for _, comment := range i.AddComments {
		actions = append(actions, fmt.Sprintf("add comment %q", comment))
	}

	if len(actions) == 0 {
		return "[no changes]"
	}
	return strings.Join(actions, ", ")
}

// Execute applies all of the requested edits, aborting on error.
func (i *pullRequestEdits) Execute(ctx context.Context, client *github.Client, data *pullRequestData) error {
	for _, comment := range i.AddComments {
		// Note: Use the Issues service since we're adding a top-level comment:
		// https://developer.github.com/v3/guides/working-with-comments/.
		_, _, err := client.Issues.CreateComment(ctx, data.OwnerLogin, data.Repo, data.PullRequest.GetNumber(), &github.IssueComment{
			Body: github.String(comment)})
		if err != nil {
			return err
		}
	}
	if len(i.AssignTo) > 0 {
		_, _, err := client.Issues.AddAssignees(ctx, data.OwnerLogin, data.Repo, data.PullRequest.GetNumber(), i.AssignTo)
		if err != nil {
			return err
		}
	}
	if i.Close {
		_, _, err := client.PullRequests.Edit(ctx, data.OwnerLogin, data.Repo, data.PullRequest.GetNumber(), &github.PullRequest{
			State: github.String("closed"),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type checkRunData struct {
	CheckRun   *github.CheckRun
	OwnerLogin string
	Repo       string
	Commit     *github.RepositoryCommit

	CodeDir string
}

func processCheckRunEvent(data *checkRunData) (*checkRunUpdates, error) {
	updates := &checkRunUpdates{
		Conclusion: "success",
		Output: &github.CheckRunOutput{
			Title:   github.String("Check passed"),
			Summary: github.String("All changed source files have valid license header."),
		},
	}
	for _, f := range data.Commit.Files {
		// Checks all source code, excludes testdata.
		if strings.Contains("/"+f.GetFilename(), "/testdata/") ||
			!checkLicenseFilenameRegexp.MatchString(filepath.Base(f.GetFilename())) &&
				!strings.EqualFold(filepath.Base(f.GetFilename()), "dockerfile") {
			continue
		}
		a, err := checkLicenseHeader(data.CodeDir, &f)
		if err != nil {
			return nil, err
		}
		if a != nil {
			updates.Output.Annotations = append(updates.Output.Annotations, a)
			if updates.Conclusion != "failure" {
				updates.Conclusion = a.GetAnnotationLevel()
			}
		}
	}
	if updates.Conclusion == "failure" {
		updates.Output.Title = github.String("Check failed")
		updates.Output.Summary = github.String("All changed source files need to have a valid license header.")
	}
	return updates, nil
}

func checkLicenseHeader(dir string, cf *github.CommitFile) (*github.CheckRunAnnotation, error) {
	file, err := os.Open(filepath.Join(dir, cf.GetFilename()))
	if err != nil {
		return nil, fmt.Errorf("check cannot open file %s: %v", cf.GetFilename(), err)
	}
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() || !licenseHeaderRegexp.Match(scanner.Bytes()) &&
		!generatedFileHeaderRegexp.Match(scanner.Bytes()) {
		return &github.CheckRunAnnotation{
			Path:            cf.Filename,
			StartLine:       github.Int(1),
			EndLine:         github.Int(1),
			AnnotationLevel: github.String("failure"),
			Message:         github.String(missingLicenseHeaderComment),
		}, nil
	}
	return nil, nil
}

type checkRunUpdates struct {
	Conclusion string
	Output     *github.CheckRunOutput
}

func (u *checkRunUpdates) Execute(ctx context.Context, client *github.Client, data *checkRunData) error {
	_, _, err := client.Checks.UpdateCheckRun(ctx, data.OwnerLogin, data.Repo, data.CheckRun.GetID(),
		github.UpdateCheckRunOptions{
			Name:        licenseHeaderCheck,
			Status:      github.String("completed"),
			Conclusion:  github.String(u.Conclusion),
			CompletedAt: &github.Timestamp{Time: time.Now()},
			Output:      u.Output,
		})
	return err
}
