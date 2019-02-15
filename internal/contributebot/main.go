// Copyright 2018 The Go Cloud Development Kit Authors
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

// contributebot is a service for keeping the Go Cloud Development Kit
// project tidy.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"gocloud.dev/pubsub"
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

	mu          sync.Mutex
	configCache map[repoKey]*repoConfigCacheEntry
}

func newWorker(sub *pubsub.Subscription, auth *gitHubAppAuth) *worker {
	return &worker{
		sub:         sub,
		auth:        auth,
		configCache: make(map[repoKey]*repoConfigCacheEntry),
	}
}

const configCacheTTL = 2 * time.Minute

type repoKey struct {
	owner string
	repo  string
}

type repoConfigCacheEntry struct {
	// On initial placement in the cache, ready will be an open channel.
	// It will be closed once the entry has been fetched and the other
	// fields are thus safe to read.
	ready <-chan struct{}

	config  repoConfig
	err     error
	fetched time.Time
}

// receive listens for events on its subscription and handles them.
func (w *worker) receive(ctx context.Context) error {
	for {
		msg, err := w.sub.Receive(ctx)
		if err != nil {
			return err
		}
		id := msg.Metadata["X-GitHub-Delivery"]
		eventType := msg.Metadata["X-GitHub-Event"]
		if eventType == "integration_installation" || eventType == "integration_installation_repositories" {
			// Deprecated event types. Ignore them in favor of supported ones.
			log.Printf("Skipped event %s of deprecated type %s", id, eventType)
			msg.Ack()
			continue
		}
		event, err := github.ParseWebHook(eventType, msg.Body)
		if err != nil {
			log.Printf("Parsing %s event %s: %v", eventType, id, err)
			msg.Ack() // We did process the message, albeit unsuccessfully.
			continue
		}
		var handleErr error
		switch event := event.(type) {
		case *github.IssuesEvent:
			handleErr = w.receiveIssueEvent(ctx, event)
		case *github.PullRequestEvent:
			handleErr = w.receivePullRequestEvent(ctx, event)
		case *github.PingEvent, *github.InstallationEvent, *github.CheckRunEvent, *github.CheckSuiteEvent, *github.PushEvent:
			// No-op.
		default:
			log.Printf("Unhandled webhook event type %s (%T) for %s", eventType, event, id)
			msg.Ack() // Although we don't know what event type this is, we did
			// process the message, by doing nothing.
			continue
		}
		if handleErr != nil {
			log.Printf("Failed processing %s event %s: %v", eventType, id, handleErr)
			// Don't ack or nack; let the message expire, and hope the error won't happen
			// when it's redelivered.
			continue
		}
		msg.Ack()
		log.Printf("Processed %s event %s", eventType, id)
	}
}

func (w *worker) receiveIssueEvent(ctx context.Context, e *github.IssuesEvent) error {
	client := w.ghClient(e.GetInstallation().GetID())

	// Pull out the interesting data from the event.
	data := &issueData{
		Action: e.GetAction(),
		Owner:  e.GetRepo().GetOwner().GetLogin(),
		Repo:   e.GetRepo().GetName(),
		Issue:  e.GetIssue(),
		Change: e.GetChanges(),
	}

	// Fetch repository configuration.
	cfg, err := w.repoConfig(ctx, client, data.Owner, data.Repo)
	if err != nil {
		return err
	}

	// Refetch the issue in case the event data is stale.
	iss, _, err := client.Issues.Get(ctx, data.Owner, data.Repo, data.Issue.GetNumber())
	if err != nil {
		return err
	}
	data.Issue = iss

	// Process the issue, deciding what actions to take (if any).
	log.Printf("Identifying actions for issue: %v", data)
	edits := processIssueEvent(cfg, data)
	log.Printf("-> %v", edits)
	// Execute the actions (if any).
	return edits.Execute(ctx, client, data)
}

func (w *worker) receivePullRequestEvent(ctx context.Context, e *github.PullRequestEvent) error {
	client := w.ghClient(e.GetInstallation().GetID())

	// Pull out the interesting data from the event.
	data := &pullRequestData{
		Action:      e.GetAction(),
		OwnerLogin:  e.GetRepo().GetOwner().GetLogin(),
		Repo:        e.GetRepo().GetName(),
		PullRequest: e.GetPullRequest(),
		Change:      e.GetChanges(),
	}

	// Fetch repository configuration.
	cfg, err := w.repoConfig(ctx, client, data.OwnerLogin, data.Repo)
	if err != nil {
		return err
	}

	// Refetch the pull request in case the event data is stale.
	pr, _, err := client.PullRequests.Get(ctx, data.OwnerLogin, data.Repo, data.PullRequest.GetNumber())
	if err != nil {
		return err
	}
	data.PullRequest = pr

	// Process the pull request, deciding what actions to take (if any).
	log.Printf("Identifying actions for pull request: %v", data)
	edits := processPullRequestEvent(cfg, data)
	log.Printf("-> %v", edits)
	// Execute the actions (if any).
	return edits.Execute(ctx, client, data)
}

// repoConfig fetches the parsed Contribute Bot configuration for the given
// repository. The result may be cached.
func (w *worker) repoConfig(ctx context.Context, client *github.Client, owner, repo string) (_ *repoConfig, err error) {
	cacheKey := repoKey{owner, repo}
	queryTime := time.Now()
	w.mu.Lock()
	ent := w.configCache[cacheKey]
	if ent != nil {
		select {
		case <-ent.ready:
			// Entry has been fully written. Check if we can use its results.
			if ent.err == nil && queryTime.Sub(ent.fetched) < configCacheTTL {
				// Cache hit; no processing necessary.
				w.mu.Unlock()
				return &ent.config, nil
			}
		default:
			// Another goroutine is currently retrieving the configuration. Block on that result.
			w.mu.Unlock()
			select {
			case <-ent.ready:
				if ent.err != nil {
					return nil, ent.err
				}
				return &ent.config, nil
			case <-ctx.Done():
				return nil, fmt.Errorf("read repository %s/%s config: %v", owner, repo, ctx.Err())
			}
		}
	}
	// Cache miss. Reserve the fetch work.
	done := make(chan struct{})
	ent = &repoConfigCacheEntry{
		ready:  done,
		config: *defaultRepoConfig(),
	}
	w.configCache[cacheKey] = ent
	w.mu.Unlock()
	defer func() {
		ent.fetched = time.Now()
		ent.err = err // err is the named return value.
		close(done)
	}()

	// Fetch the configuration from the repository.
	content, _, response, err := client.Repositories.GetContents(ctx, owner, repo, ".contributebot", nil)
	if response != nil && response.StatusCode == http.StatusNotFound {
		// File not found. Use default configuration.
		return &ent.config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read repository %s/%s config: %v", owner, repo, err)
	}
	data, err := content.GetContent()
	if err != nil {
		return nil, fmt.Errorf("read repository %s/%s config: %v", owner, repo, err)
	}
	if err := json.Unmarshal([]byte(data), &ent.config); err != nil {
		return nil, fmt.Errorf("read repository %s/%s config: %v", owner, repo, err)
	}
	return &ent.config, nil
}

// isClosed tests whether a channel is closed without blocking.
func isClosed(c <-chan struct{}) bool {
	select {
	case _, ok := <-c:
		return !ok
	default:
		return false
	}
}

// ghClient creates a GitHub client authenticated for the given installation.
func (w *worker) ghClient(installID int64) *github.Client {
	c := github.NewClient(&http.Client{Transport: w.auth.forInstall(installID)})
	c.UserAgent = userAgent
	return c
}

// ServeHTTP serves a page explaining that this port is only open for health checks.
func (w *worker) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(resp, req)
		return
	}
	const responseData = `<!DOCTYPE html>
<title>Go Cloud Development Kit Contribute Bot Worker</title>
<h1>Go Cloud Development Kit Contribute Bot Worker</h1>
<p>This HTTP port is only open to serve health checks.</p>`
	resp.Header().Set("Content-Length", fmt.Sprint(len(responseData)))
	resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(resp, responseData)
}

func (w *worker) CheckHealth() error {
	return w.auth.CheckHealth()
}
