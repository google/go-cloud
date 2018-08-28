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

// contributebot is a service for keeping the Go Cloud project tidy.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"

	"cloud.google.com/go/pubsub"
	"github.com/google/go-github/github"
)

const userAgent = "google/go-cloud Contribute Bot"

type flagConfig struct {
	project      string
	subscription string
	gitHubAppID  int64
	keyPath      string
}

func main() {
	addr := flag.String("listen", ":8080", "address to listen for health checks")
	var cfg flagConfig
	flag.StringVar(&cfg.project, "project", "", "GCP project for topic")
	flag.StringVar(&cfg.subscription, "subscription", "contributebot-github-events", "subscription name inside project")
	flag.Int64Var(&cfg.gitHubAppID, "github_app", 0, "GitHub application ID")
	flag.StringVar(&cfg.keyPath, "github_key", "", "path to GitHub application private key")
	flag.Parse()
	if cfg.project == "" || cfg.gitHubAppID == 0 || cfg.keyPath == "" {
		fmt.Fprintln(os.Stderr, "contributebot: must specify -project, -github_app, and -github_key")
		os.Exit(2)
	}

	ctx := context.Background()
	w, server, cleanup, err := setup(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	log.Printf("Serving health checks at %s", *addr)
	go server.ListenAndServe(*addr, w)
	log.Fatal(w.receive(ctx))
}

// worker contains the connections used by this server.
type worker struct {
	sub  *pubsub.Subscription
	auth *gitHubAppAuth
}

// receive listens for events on its subscription and handles them.
func (w *worker) receive(ctx context.Context) error {
	return w.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		id := msg.Attributes["X-GitHub-Delivery"]
		eventType := msg.Attributes["X-GitHub-Event"]
		if eventType == "integration_installation" || eventType == "integration_installation_repositories" {
			// Deprecated event types. Ignore them in favor of supported ones.
			log.Printf("Skipped event %s of deprecated type %s", id, eventType)
			msg.Ack()
			return
		}
		event, err := github.ParseWebHook(eventType, msg.Data)
		if err != nil {
			log.Printf("Parsing %s event %s: %v", eventType, id, err)
			msg.Nack()
			return
		}
		var handleErr error
		switch event := event.(type) {
		case *github.IssuesEvent:
			handleErr = w.receiveIssueEvent(ctx, event)
		case *github.PingEvent, *github.InstallationEvent, *github.PullRequestEvent, *github.CheckSuiteEvent:
			// No-op.
		default:
			log.Printf("Unhandled webhook event type %s (%T) for %s", eventType, event, id)
			msg.Nack()
			return
		}
		if handleErr != nil {
			log.Printf("Failed processing %s event %s: %v", eventType, id, handleErr)
			msg.Nack()
			return
		}
		msg.Ack()
		log.Printf("Processed %s event %s", eventType, id)
	})
}

func (w *worker) receiveIssueEvent(ctx context.Context, e *github.IssuesEvent) error {

	data := issueRuleData{
		action:  e.GetAction(),
		owner:   e.GetRepo().GetOwner().GetLogin(),
		repo:    e.GetRepo().GetName(),
		issue:   e.GetIssue(),
		changes: e.GetChanges(),
	}

	// Check conditions to see if issue data is needed.
	// No need to consume API quota if events aren't relevant.
	var toRun []issueRule
	for _, rule := range allIssueRules {
		if rule.Condition(data) {
			toRun = append(toRun, rule)
		}
	}
	if len(toRun) == 0 {
		log.Printf("No issue rules matched %v", data)
		return nil
	}

	// Retrieve the current issue state.
	client := w.ghClient(e.GetInstallation().GetID())
	iss, _, err := client.Issues.Get(ctx, data.owner, data.repo, data.issue.GetNumber())
	if err != nil {
		return err
	}
	data.issue = iss

	// Execute relevant rules.
	ok := true
	for _, r := range toRun {
		// Recheck Condition with fresh issue data.
		if !r.Condition(data) {
			continue
		}
		if err := r.Run(ctx, client, data); err != nil {
			ok = false
			log.Printf("  Issue rule %q failed on %v: %v", r.Name(), data, err)
		} else {
			log.Printf("  Issue rule %q succeeded on %v", r.Name(), data)
		}
	}
	if !ok {
		return fmt.Errorf("one or more rules failed for %v", data)
	}
	log.Printf("Applied %d relevant issue rule(s) on %v successfully", len(toRun), data)
	return nil
}

// issueRuleData is the information passed to an issue rule.
type issueRuleData struct {
	action  string
	repo    string
	owner   string
	issue   *github.Issue
	changes *github.EditChange
}

func (ird issueRuleData) String() string {
	return fmt.Sprintf("[%s %s/%s#%d]", ird.action, ird.owner, ird.repo, ird.issue.GetNumber())
}

// issueRule defines a rule about what to do for a given event.
type issueRule interface {
	// Name returns the rule's name for error messages.
	Name() string
	// Condition reports true if the event is "interesting" to this rule,
	// meaning that the worker should fetch the latest information about
	// the issue and call the Run function.
	Condition(issueRuleData) bool
	// Run executes the rule. The issue data is retrieved before any
	// rules are executed, so may be ever-so-slightly stale.
	Run(context.Context, *github.Client, issueRuleData) error
}

var allIssueRules = []issueRule{
	removeInProgressLabelFromClosedIssues{},
	checkIssueTitleFormat{},
}

// Remove "in progress" label from closed issues.
type removeInProgressLabelFromClosedIssues struct{}

const inProgressLabel = "in progress"

func (removeInProgressLabelFromClosedIssues) Name() string {
	return "remove in progress label from closed"
}
func (removeInProgressLabelFromClosedIssues) Condition(data issueRuleData) bool {
	return data.action == "closed" && hasLabel(data.issue, inProgressLabel)
}
func (removeInProgressLabelFromClosedIssues) Run(ctx context.Context, client *github.Client, data issueRuleData) error {
	num := data.issue.GetNumber()
	_, err := client.Issues.RemoveLabelForIssue(ctx, data.owner, data.repo, num, inProgressLabel)
	if err != nil {
		return err
	}
	return nil
}

// Check format of issue titles.
type checkIssueTitleFormat struct{}

var issueTitleRegexp = regexp.MustCompile("^[a-z0-9/]+: .*$")

const issueTitleComment = "Please edit the title of this issue with the name of the affected package, followed by a colon, followed by a short summary of the issue. Example: \"blob/gcsblob: not blobby enough\"."

func (checkIssueTitleFormat) Name() string {
	return "check issue title format"
}
func (checkIssueTitleFormat) Condition(data issueRuleData) bool {
	// Add a comment if the title doesn't match our regexp, and it's a new issue,
	// or an issue whose title has just been modified.
	if issueTitleRegexp.MatchString(data.issue.GetTitle()) {
		return false
	}
	if data.action == "opened" {
		return true
	}
	if data.action != "edited" {
		return false
	}
	return data.changes != nil && data.changes.Title != nil && *data.changes.Title.From != data.issue.GetTitle()
}
func (checkIssueTitleFormat) Run(ctx context.Context, client *github.Client, data issueRuleData) error {
	num := data.issue.GetNumber()
	_, _, err := client.Issues.CreateComment(ctx, data.owner, data.repo, num, &github.IssueComment{
		Body: github.String(issueTitleComment)})
	if err != nil {
		return err
	}
	return nil
}

// ghClient creates a GitHub client authenticated for the given installation.
func (w *worker) ghClient(installID int64) *github.Client {
	c := github.NewClient(&http.Client{Transport: w.auth.forInstall(installID)})
	c.UserAgent = userAgent
	return c
}

func hasLabel(iss *github.Issue, label string) bool {
	for i := range iss.Labels {
		if iss.Labels[i].GetName() == label {
			return true
		}
	}
	return false
}

// ServeHTTP serves a page explaining that this port is only open for health checks.
func (w *worker) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(resp, req)
		return
	}
	const responseData = `<!DOCTYPE html>
<title>Go Cloud Contribute Bot Worker</title>
<h1>Go Cloud Contribute Bot Worker</h1>
<p>This HTTP port is only open to serve health checks.</p>`
	resp.Header().Set("Content-Length", fmt.Sprint(len(responseData)))
	resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(resp, responseData)
}

func (w *worker) CheckHealth() error {
	return w.auth.CheckHealth()
}
