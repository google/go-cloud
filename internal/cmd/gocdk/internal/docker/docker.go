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
	// ID is the randomly generated ID of the image.
	// See https://windsock.io/explaining-docker-image-ids/
	ID string
	// Repository is the name component of a Docker image reference.
	// It may be empty if the image has no tags.
	// See https://godoc.org/github.com/docker/distribution/reference
	Repository string
	// Tag is the tag component of a Docker image reference.
	// It will be empty if the image has no tags.
	// See https://godoc.org/github.com/docker/distribution/reference
	Tag string
	// Digest is the content-based hash of the image.
	// It may be empty.
	Digest string
	// CreatedAt is the time the image was built.
	CreatedAt time.Time
}

// ListImages lists images the Docker daemon has stored locally that match filterRef.
func (c *Client) ListImages(ctx context.Context, filterRef string) ([]*Image, error) {
	args := []string{"images", "--digests", "--format={{json .}}"}
	if filterRef != "" {
		args = append(args, filterRef)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = c.env
	out, err := run(cmd)
	if err != nil {
		return nil, xerrors.Errorf("list docker images: %w", err)
	}
	lf := []byte("\n")
	out = bytes.TrimSuffix(out, lf)
	var images []*Image
	for _, line := range bytes.Split(out, lf) {
		if len(line) == 0 {
			continue
		}
		var img struct {
			ID         string
			Repository string
			Tag        string
			Digest     string
			CreatedAt  string
		}
		if err := json.Unmarshal(line, &img); err != nil {
			return nil, xerrors.Errorf("list docker images: parse output: %w", err)
		}
		createdAt, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", img.CreatedAt)
		images = append(images, &Image{
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

// Start starts a Docker container and returns its ID.
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
	out, err := run(cmd)
	if err != nil {
		return "", xerrors.Errorf("docker run %s: %w", imageRef, err)
	}
	out = bytes.TrimSuffix(out, []byte("\n"))
	if len(out) == 0 {
		return "", xerrors.Errorf("docker run %s: docker did not return a container ID", imageRef)
	}
	return string(out), nil
}

// Build builds a Docker image from a source directory, optionally tagging it
// with the given image references. Output is streamed to the buildOutput writer.
func (c *Client) Build(ctx context.Context, imageRefs []string, dir string, buildOutput io.Writer) error {
	args := []string{"build"}
	for _, r := range imageRefs {
		args = append(args, "-t="+r)
	}
	args = append(args, "--", dir)
	cmd := exec.CommandContext(ctx, "docker", args...)
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
		return xerrors.Errorf("docker push: %w", cmdError(err, out))
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

// Login stores the credentials for the given remote registry. This is used for
// Docker push and pull.
func (c *Client) Login(ctx context.Context, registry, username, password string) error {
	cmd := exec.CommandContext(ctx, "docker", "login", "--username="+username, "--password-stdin", "--", registry)
	cmd.Stdin = strings.NewReader(password)
	if out, err := cmd.CombinedOutput(); err != nil {
		return xerrors.Errorf("docker login: %w", cmdError(err, out))
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

// ImageRefRegistry parses the registry (everything before the first slash) from
// a Docker image reference or name.
func ImageRefRegistry(s string) string {
	name, _, _ := ParseImageRef(s)
	i := strings.IndexByte(name, '/')
	if i == -1 {
		return ""
	}
	return name[:i]
}

// run runs a command, capturing its stdout and stderr, and returns the stdout
// output. If the program exits with a failure status code, the error will
// contain the stderr.
func run(cmd *exec.Cmd) ([]byte, error) {
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	return out, cmdError(err, stderr.Bytes())
}

// cmdError wraps an error with stderr output as needed.
func cmdError(err error, stderr []byte) error {
	if err == nil || len(stderr) == 0 {
		return err
	}
	// TODO(light): Maybe wrap err?
	return xerrors.New(string(stderr))
}

func isTagChar(c rune) bool {
	return 'a' <= c && c <= 'z' ||
		'A' <= c && c <= 'Z' ||
		'0' <= c && c <= '9' ||
		c == '_' || c == '-' || c == '.'
}
