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

package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/sys/unix"
)

// syncParams is the set of parameters to syncPullRequest.
type syncParams struct {
	BaseOwner  string
	BaseRepo   string
	BaseBranch string
	PRNumber   int

	HeadOwner  string
	HeadRepo   string
	HeadBranch string
}

// syncPullRequest merges the latest changes on the base branch into the head branch.
// syncPullRequest only returns errors if the interactions with GitHub fail.
func syncPullRequest(ctx context.Context, gitPath string, auth *gitHubInstallAuth, client *github.Client, params syncParams) error {
	tok, err := auth.fetchToken(ctx)
	if err != nil {
		// If this doesn't work, nothing else will.
		return err
	}

	// Perform the merge. The Git authentication scheme is described at
	// https://developer.github.com/apps/building-github-apps/authenticating-with-github-apps/#http-based-git-access-by-an-installation
	headURL := &url.URL{
		Scheme: "https",
		User:   url.UserPassword("x-access-token", tok),
		Host:   "github.com",
		Path:   fmt.Sprintf("%s/%s.git", params.HeadOwner, params.HeadRepo),
	}
	baseURL := &url.URL{
		Scheme: "https",
		User:   url.UserPassword("x-access-token", tok),
		Host:   "github.com",
		Path:   fmt.Sprintf("%s/%s.git", params.BaseOwner, params.BaseRepo),
	}
	err = gitSync(ctx, gitPath,
		remoteGitBranch{repo: headURL.String(), branch: params.HeadBranch},
		remoteGitBranch{repo: baseURL.String(), branch: params.BaseBranch})
	if err == nil {
		// Success is much simpler than failure. Return early.
		return nil
	}

	// Log the error in case there's a bigger issue.
	prName := fmt.Sprintf("%s/%s#%d", params.BaseOwner, params.BaseRepo, params.PRNumber)
	log.Printf("Could not sync %s: %v", prName, err)

	// Add comment to PR indicating failure.
	msg := fmt.Sprintf("I was unable to merge %s:%s into %s:%s. This is likely due to merge conflicts. A project maintainer must merge the branches manually.", params.BaseOwner, params.BaseBranch, params.HeadOwner, params.HeadBranch)
	_, _, err = client.Issues.CreateComment(ctx,
		params.BaseOwner, params.BaseRepo, params.PRNumber,
		&github.IssueComment{Body: github.String(msg)})
	if err != nil {
		return fmt.Errorf("add merge failure comment to %s: %v", prName, err)
	}
	return nil
}

// remoteGitBranch identifies a remote Git branch.
type remoteGitBranch struct {
	repo   string // URL or path to repository, like "https://github.com/octocat/example.git"
	branch string // name of branch, like "master"
}

// gitSync merges the changes on branch src into branch dst. It performs the Git
// operations locally. If the src branch's head commit is an ancestor of the dst
// branch (i.e. all changes have already been merged in), then the head branch
// will remain unchanged.
func gitSync(ctx context.Context, gitPath string, dst, src remoteGitBranch) error {
	if src.branch == "" || strings.HasPrefix(src.branch, "-") {
		return fmt.Errorf("invalid source branch %q", src.branch)
	}
	if dst.branch == "" || strings.HasPrefix(dst.branch, "-") {
		return fmt.Errorf("invalid destination branch %q", dst.branch)
	}
	tempDir, err := ioutil.TempDir("", "contributebot_sync")
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("Failed to clean up temporary directory %s after sync: %v", tempDir, err)
		}
	}()

	// Clone to temporary directory.
	err = runCommand(ctx, tempDir, gitPath, []string{
		"clone",
		"--quiet",
		"--branch", dst.branch,
		"--",
		dst.repo,
		"."})
	if err != nil {
		return err
	}

	// Merge in upstream.
	err = runCommand(ctx, tempDir, gitPath, []string{
		// Use Contribute Bot identity for created commits.
		"-c", "user.name=Contribute Bot",
		"-c", "user.email=noreply@gocloud.dev",
		// Use pull: fetch + merge.
		"pull",
		"--quiet",
		// Always create a merge commit.
		"--commit",
		"--no-edit",
		// Source for merge.
		"--",
		src.repo,
		"refs/heads/" + src.branch})
	if err != nil {
		return err
	}

	// Push the resulting commit back to the head ref.
	return runCommand(ctx, tempDir, gitPath, []string{
		"push",
		"--quiet",
		"origin",
		"HEAD:refs/heads/" + dst.branch})
}

// runCommand starts a subprocess and waits for it to finish. If the subprocess
// exits with failure, the returned error's message will include the combined
// stdout and stderr. If the context is cancelled, then runCommand sends the
// subprocess SIGTERM (this differs from CommandContext, which sends SIGKILL).
func runCommand(ctx context.Context, dir string, exe string, args []string) error {
	c := exec.Command(exe, args...)
	c.Dir = dir
	out := new(bytes.Buffer)
	c.Stdout = out
	c.Stderr = out
	if err := c.Start(); err != nil {
		return err
	}

	// Wait for subprocess to finish. While doing so, listen for context cancellation.
	waitDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Ignore error: if we can't signal, it's likely because the process already exited.
			_ = c.Process.Signal(unix.SIGTERM)
		case <-waitDone:
		}
		// No further synchronization back to the main goroutine, because signal is uninterruptable and
		// there's no meaningful work after the waitDone signal is received.
	}()
	err := c.Wait()
	close(waitDone)
	if err != nil {
		// Return an error with the stderr in the message.
		exeName := filepath.Base(exe)
		stderr := bytes.TrimSuffix(out.Bytes(), []byte{'\n'})
		if len(stderr) == 0 {
			return fmt.Errorf("run %s: %v", exeName, err)
		}
		if _, isExit := err.(*exec.ExitError); isExit {
			// Exit errors don't add useful details if output is not empty.
			// Just show the output.
			if bytes.IndexByte(stderr, '\n') == -1 {
				// Collapse into single line.
				return fmt.Errorf("run %s: %s", exeName, stderr)
			}
			return fmt.Errorf("run %s:\n%s", exeName, stderr)
		}
		return fmt.Errorf("run %s: %v\n%s", exeName, err, stderr)
	}
	return nil
}
