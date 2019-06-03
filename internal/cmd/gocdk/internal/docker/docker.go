// Copyright 2019 The Go Cloud Development Kit Authors
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

// Package docker provides a client for interacting with the local Docker
// daemon. This currently shells out to the Docker CLI, but could use the HTTP
// API directly in the future.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// Client is a client for a Docker daemon. The zero value is a Docker client
// that uses the current process's environment. Methods on Client are safe to
// call concurrently from multiple goroutines.
type Client struct {
	env []string
}

// New creates a new client with the given environment variables.
// Calling docker.New(nil) is the same as new(docker.Client) and uses the
// current process's environment.
func New(env []string) *Client {
	return &Client{env: env}
}

// Image stores metadata about a Docker image.
type Image struct {
	ID         string
	Repository string
	Tag        string
	Digest     string
	CreatedAt  time.Time
}

// ListImages list images the Docker daemon has stored locally that match filterRef.
func (c *Client) ListImages(ctx context.Context, filterRef string) ([]Image, error) {
	args := []string{"images", "--digests", "--format={{json .}}"}
	if filterRef != "" {
		args = append(args, filterRef)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = c.env
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return nil, xerrors.Errorf("docker images:\n%s", stderr)
		}
		return nil, xerrors.Errorf("docker images: %w", err)
	}
	lf := []byte("\n")
	out = bytes.TrimSuffix(out, lf)
	var images []Image
	for _, line := range bytes.Split(out, lf) {
		var img struct {
			ID         string
			Repository string
			Tag        string
			Digest     string
			CreatedAt  string
		}
		if err := json.Unmarshal(line, &img); err != nil {
			return nil, xerrors.Errorf("docker images: parse output: %w", err)
		}
		createdAt, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", img.CreatedAt)
		images = append(images, Image{
			ID:         cleanNone(img.ID),
			Repository: cleanNone(img.Repository),
			Tag:        cleanNone(img.Tag),
			Digest:     cleanNone(img.Digest),
			CreatedAt:  createdAt,
		})
	}
	return images, nil
}

func cleanNone(s string) string {
	if s == "<none>" {
		return ""
	}
	return s
}

// RunOptions holds optional parameters for starting a Docker container.
type RunOptions struct {
	Args         []string
	Env          []string
	RemoveOnExit bool
	Publish      []string
}

// Start starts a Docker container. It does not wait for the container to stop.
// Start returns the container's ID on success.
func (c *Client) Start(ctx context.Context, imageRef string, opts *RunOptions) (string, error) {
	args := []string{"run", "--detach"}
	if opts != nil {
		if opts.RemoveOnExit {
			args = append(args, "--rm")
		}
		for _, e := range opts.Env {
			args = append(args, "--env", e)
		}
		for _, p := range opts.Publish {
			args = append(args, "--publish", p)
		}
	}
	args = append(args, "--", imageRef)
	if opts != nil {
		args = append(args, opts.Args...)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = c.env
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", xerrors.Errorf("docker run %s:\n%s", imageRef, stderr)
		}
		return "", xerrors.Errorf("docker run %s: %w", imageRef, err)
	}
	out = bytes.TrimSuffix(out, []byte("\n"))
	if len(out) == 0 {
		return "", xerrors.Errorf("docker run %s: docker did not return a container ID", imageRef)
	}
	return string(out), nil
}

// Build builds a Docker image from a source directory, optionally tagging it
// with the given image reference. Output is streamed to the buildOutput writer.
func (c *Client) Build(ctx context.Context, imageRef string, dir string, buildOutput io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "build", "-t="+imageRef, "--", dir)
	cmd.Stdout = buildOutput
	cmd.Stderr = buildOutput
	cmd.Env = c.env
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("docker build: %w", err)
	}
	return nil
}

// Tag adds a new tag to a given image reference.
func (c *Client) Tag(ctx context.Context, srcImageRef, dstImageRef string) error {
	cmd := exec.CommandContext(ctx, "docker", "tag", "--", srcImageRef, dstImageRef)
	cmd.Env = c.env
	if out, err := cmd.CombinedOutput(); err != nil {
		if len(out) > 0 {
			return xerrors.Errorf("docker tag:\n%s", out)
		}
		return xerrors.Errorf("docker push: %w", err)
	}
	return nil
}

// Push pushes the image named by imageRef from the local Docker daemon to its
// remote registry, as determined by the reference's repository name. Progress
// information is written to the progressOutput writer.
func (c *Client) Push(ctx context.Context, imageRef string, progressOutput io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "push", "--", imageRef)
	cmd.Stdout = progressOutput
	cmd.Stderr = progressOutput
	cmd.Env = c.env
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("docker push: %w", err)
	}
	return nil
}

// ParseImageRef parses a Docker image reference, as documented in
// https://godoc.org/github.com/docker/distribution/reference. It permits some
// looseness in characters, and in particular, permits the empty name form
// ":foo". It is guaranteed that name + tag + digest == s.
func ParseImageRef(s string) (name, tag, digest string) {
	if i := strings.LastIndexByte(s, '@'); i != -1 {
		s, digest = s[:i], s[i:]
	}
	i := strings.LastIndexFunc(s, func(c rune) bool { return !isTagChar(c) })
	if i == -1 || s[i] != ':' {
		return s, "", digest
	}
	return s[:i], s[i:], digest
}

func isTagChar(c rune) bool {
	return 'a' <= c && c <= 'z' ||
		'A' <= c && c <= 'Z' ||
		'0' <= c && c <= '9' ||
		c == '_' || c == '-' || c == '.'
}
