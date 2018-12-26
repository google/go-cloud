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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitSync(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Can't find system Git:", err)
	}
	c := exec.Command(gitPath, "--version")
	versionBuf := new(strings.Builder)
	c.Stdout = versionBuf
	if err := c.Run(); err != nil {
		t.Fatal("Getting Git version info:", err)
	}
	t.Logf("Git version: %s", strings.TrimSuffix(versionBuf.String(), "\n"))

	// Test suite:
	t.Run("BranchesEven", func(t *testing.T) { testGitSyncBranchesEven(t, gitPath) })
	t.Run("BranchAhead", func(t *testing.T) { testGitSyncBranchAhead(t, gitPath) })
	t.Run("BranchBehind", func(t *testing.T) { testGitSyncBranchBehind(t, gitPath) })
	t.Run("Diverge", func(t *testing.T) { testGitSyncDiverge(t, gitPath) })
}

func testGitSyncBranchesEven(t *testing.T, gitPath string) {
	tempDir, err := ioutil.TempDir("", "contributebot_sync_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Cleaning up %s: %v", tempDir, err)
		}
	}()

	// Create a repository A with a single commit on master.
	ctx := context.Background()
	if err := runCommand(ctx, tempDir, gitPath, []string{"init", "repoA"}); err != nil {
		t.Fatal(err)
	}
	repoRootA := filepath.Join(tempDir, "repoA")
	if err := dummyFile(repoRootA, "file.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"add", "file.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"commit", "--quiet", "-m", "first"}); err != nil {
		t.Fatal(err)
	}
	master, err := revParse(repoRootA, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootA, gitPath); err != nil {
		t.Fatal(err)
	}

	// Clone repository A to repository B.
	// Create a feature branch that points to the same commit as master.
	if err := runCommand(ctx, tempDir, gitPath, []string{"clone", "--quiet", "repoA", "repoB"}); err != nil {
		t.Fatal(err)
	}
	repoRootB := filepath.Join(tempDir, "repoB")
	if err := runCommand(ctx, repoRootB, gitPath, []string{"checkout", "--quiet", "-b", "feature"}); err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootB, gitPath); err != nil {
		t.Fatal(err)
	}

	// Run sync.
	err = gitSync(ctx, gitPath,
		remoteGitBranch{repo: repoRootB, branch: "feature"}, // destination
		remoteGitBranch{repo: repoRootA, branch: "master"})  // source
	if err != nil {
		t.Error("gitSync:", err)
	}

	// Verify that feature branch still points to same commit.
	if got, err := revParse(repoRootB, gitPath, "refs/heads/feature"); err != nil {
		t.Error(err)
	} else if got != master {
		t.Error("feature now points to a different commit; want to keep branch the same")
	}
}

func testGitSyncBranchAhead(t *testing.T, gitPath string) {
	tempDir, err := ioutil.TempDir("", "contributebot_sync_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Cleaning up %s: %v", tempDir, err)
		}
	}()

	// Create a repository A with a single commit on master.
	ctx := context.Background()
	if err := runCommand(ctx, tempDir, gitPath, []string{"init", "repoA"}); err != nil {
		t.Fatal(err)
	}
	repoRootA := filepath.Join(tempDir, "repoA")
	if err := dummyFile(repoRootA, "file1.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"add", "file1.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"commit", "--quiet", "-m", "first"}); err != nil {
		t.Fatal(err)
	}
	commit1, err := revParse(repoRootA, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootA, gitPath); err != nil {
		t.Fatal(err)
	}

	// Clone repository A to repository B.
	// Create a feature branch based on master and create a second commit.
	if err := runCommand(ctx, tempDir, gitPath, []string{"clone", "--quiet", "repoA", "repoB"}); err != nil {
		t.Fatal(err)
	}
	repoRootB := filepath.Join(tempDir, "repoB")
	if err := runCommand(ctx, repoRootB, gitPath, []string{"checkout", "--quiet", "-b", "feature", "HEAD"}); err != nil {
		t.Fatal(err)
	}
	if err := dummyFile(repoRootB, "file2.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootB, gitPath, []string{"add", "file2.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootB, gitPath, []string{"commit", "--quiet", "-m", "second"}); err != nil {
		t.Fatal(err)
	}
	commit2, err := revParse(repoRootB, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootB, gitPath); err != nil {
		t.Fatal(err)
	}

	// Run sync.
	err = gitSync(ctx, gitPath,
		remoteGitBranch{repo: repoRootB, branch: "feature"}, // destination
		remoteGitBranch{repo: repoRootA, branch: "master"})  // source
	if err != nil {
		t.Error("gitSync:", err)
	}

	// Verify that feature branch points to commit 2.
	if got, err := revParse(repoRootB, gitPath, "refs/heads/feature"); err != nil {
		t.Error(err)
	} else if got == commit1 {
		t.Error("feature = commit 1; want commit 2")
	} else if got != commit2 {
		t.Error("feature = <unknown commit>; want commit 2")
	}
}

func testGitSyncBranchBehind(t *testing.T, gitPath string) {
	tempDir, err := ioutil.TempDir("", "contributebot_sync_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Cleaning up %s: %v", tempDir, err)
		}
	}()

	// Create a repository A with a single commit on master.
	ctx := context.Background()
	if err := runCommand(ctx, tempDir, gitPath, []string{"init", "repoA"}); err != nil {
		t.Fatal(err)
	}
	repoRootA := filepath.Join(tempDir, "repoA")
	if err := dummyFile(repoRootA, "file1.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"add", "file1.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"commit", "--quiet", "-m", "first"}); err != nil {
		t.Fatal(err)
	}
	commit1, err := revParse(repoRootA, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := dummyFile(repoRootA, "file2.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"add", "file2.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"commit", "--quiet", "-m", "second"}); err != nil {
		t.Fatal(err)
	}
	commit2, err := revParse(repoRootA, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootA, gitPath); err != nil {
		t.Fatal(err)
	}

	// Clone repository A to repository B.
	// Create a feature branch pointing to the first commit on master.
	if err := runCommand(ctx, tempDir, gitPath, []string{"clone", "--quiet", "repoA", "repoB"}); err != nil {
		t.Fatal(err)
	}
	repoRootB := filepath.Join(tempDir, "repoB")
	if err := runCommand(ctx, repoRootB, gitPath, []string{"checkout", "--quiet", "-b", "feature", "HEAD^"}); err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootB, gitPath); err != nil {
		t.Fatal(err)
	}

	// Run sync.
	err = gitSync(ctx, gitPath,
		remoteGitBranch{repo: repoRootB, branch: "feature"}, // destination
		remoteGitBranch{repo: repoRootA, branch: "master"})  // source
	if err != nil {
		t.Error("gitSync:", err)
	}

	// Verify that feature branch points to commit 2.
	if got, err := revParse(repoRootB, gitPath, "refs/heads/feature"); err != nil {
		t.Error(err)
	} else if got == commit1 {
		t.Error("feature = commit 1; want commit 2")
	} else if got != commit2 {
		t.Error("feature = <unknown commit>; want commit 2")
	}
}

func testGitSyncDiverge(t *testing.T, gitPath string) {
	tempDir, err := ioutil.TempDir("", "contributebot_sync_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Cleaning up %s: %v", tempDir, err)
		}
	}()

	// Create a repository A with two commits on master.
	ctx := context.Background()
	if err := runCommand(ctx, tempDir, gitPath, []string{"init", "repoA"}); err != nil {
		t.Fatal(err)
	}
	repoRootA := filepath.Join(tempDir, "repoA")
	if err := dummyFile(repoRootA, "file1.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"add", "file1.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"commit", "--quiet", "-m", "first"}); err != nil {
		t.Fatal(err)
	}
	commit1, err := revParse(repoRootA, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := dummyFile(repoRootA, "file2.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"add", "file2.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootA, gitPath, []string{"commit", "--quiet", "-m", "second"}); err != nil {
		t.Fatal(err)
	}
	commit2, err := revParse(repoRootA, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootA, gitPath); err != nil {
		t.Fatal(err)
	}

	// Clone repository A to repository B.
	// Create a feature branch based on the first commit on master and create a third commit.
	if err := runCommand(ctx, tempDir, gitPath, []string{"clone", "--quiet", "repoA", "repoB"}); err != nil {
		t.Fatal(err)
	}
	repoRootB := filepath.Join(tempDir, "repoB")
	if err := runCommand(ctx, repoRootB, gitPath, []string{"checkout", "--quiet", "-b", "feature", "HEAD^"}); err != nil {
		t.Fatal(err)
	}
	if err := dummyFile(repoRootB, "file3.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootB, gitPath, []string{"add", "file3.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, repoRootB, gitPath, []string{"commit", "--quiet", "-m", "third"}); err != nil {
		t.Fatal(err)
	}
	commit3, err := revParse(repoRootB, gitPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := makeRepoBare(repoRootB, gitPath); err != nil {
		t.Fatal(err)
	}

	// Run sync.
	err = gitSync(ctx, gitPath,
		remoteGitBranch{repo: repoRootB, branch: "feature"}, // destination
		remoteGitBranch{repo: repoRootA, branch: "master"})  // source
	if err != nil {
		t.Error("gitSync:", err)
	}

	// Verify that feature branch points to a new commit.
	if got, err := revParse(repoRootB, gitPath, "refs/heads/feature"); err != nil {
		t.Error(err)
	} else if got == commit1 {
		t.Error("feature = commit 1; want new merge commit")
	} else if got == commit2 {
		t.Error("feature = commit 2; want new merge commit")
	} else if got == commit3 {
		t.Error("feature = commit 3; want new merge commit")
	}
	// Verify that parent 1 of the feature branch is commit 3.
	if got, err := revParse(repoRootB, gitPath, "refs/heads/feature^1"); err != nil {
		t.Error(err)
	} else if got == commit1 {
		t.Error("feature^1 = commit 1; want commit 3")
	} else if got == commit2 {
		t.Error("feature^1 = commit 2; want commit 3")
	} else if got != commit3 {
		t.Error("feature^1 = <unknown commit>; want commit 3")
	}
	// Verify that parent 2 of the feature branch is commit 3.
	if got, err := revParse(repoRootB, gitPath, "refs/heads/feature^2"); err != nil {
		t.Error(err)
	} else if got == commit1 {
		t.Error("feature^2 = commit 1; want commit 2")
	} else if got == commit3 {
		t.Error("feature^2 = commit 3; want commit 2")
	} else if got != commit2 {
		t.Error("feature^2 = <unknown commit>; want commit 2")
	}
}

// revParse obtains the Git commit hash of the named revision.
// More details at git-rev-parse(1) and revision syntax at gitrevisions(7).
func revParse(dir string, gitPath string, rev string) (string, error) {
	c := exec.Command(gitPath, "rev-parse", "-q", "--verify", "--revs-only", rev)
	c.Dir = dir
	buf := new(strings.Builder)
	c.Stdout = buf
	err := c.Run()
	return strings.TrimSuffix(buf.String(), "\n"), err
}

// makeRepoBare converts a repository with a working copy into a bare one.
// This uses the method described at https://git.wiki.kernel.org/index.php/GitFaq#How_do_I_make_existing_non-bare_repository_bare.3F
func makeRepoBare(dir string, gitPath string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("make repo %s bare: %v", dir, err)
	}
	tempDir := dir + ".git"
	if err := os.Rename(filepath.Join(dir, ".git"), tempDir); err != nil {
		return fmt.Errorf("make repo %s bare: %v", dir, err)
	}
	err = runCommand(context.Background(), tempDir, gitPath, []string{"--git-dir=" + tempDir, "config", "core.bare", "true"})
	if err != nil {
		return fmt.Errorf("make repo %s bare: %v", dir, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("make repo %s bare: %v", dir, err)
	}
	if err := os.Rename(tempDir, dir); err != nil {
		return fmt.Errorf("make repo %s bare: %v", dir, err)
	}
	return nil
}

func dummyFile(root, slashpath string) error {
	path := filepath.Join(root, filepath.FromSlash(slashpath))
	return ioutil.WriteFile(path, []byte("Hello, World!\n"), 0666)
}
