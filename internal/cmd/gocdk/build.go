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

package main

import (
	"bytes"
	"context"
	"flag"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

func build(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "build")
	list := f.Bool("list", false, "display Docker images of this project")
	ref := f.String("t", ":latest", "name and/or tag in the form `name[:tag] OR :tag`")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk build: %w", err)
	}
	if *list {
		if err := listBuilds(ctx, pctx, f.Args()); err != nil {
			return xerrors.Errorf("gocdk build: %w", err)
		}
		return nil
	}
	moduleRoot, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		return xerrors.Errorf("gocdk build: %w", err)
	}
	if strings.HasPrefix(*ref, ":") {
		imageName, err := moduleDockerImageName(moduleRoot)
		if err != nil {
			return xerrors.Errorf("gocdk build: %w", err)
		}
		*ref = imageName + *ref
	}
	c := exec.CommandContext(ctx, "docker", "build", "--tag", *ref, ".")
	c.Dir = moduleRoot
	c.Stdout = pctx.stdout
	c.Stderr = pctx.stderr
	if err := c.Run(); err != nil {
		return xerrors.Errorf("gocdk build: %w", err)
	}
	return nil
}

func listBuilds(ctx context.Context, pctx *processContext, args []string) error {
	if len(args) != 0 {
		return usagef("gocdk build --list")
	}
	moduleRoot, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		return xerrors.Errorf("list builds: %w", err)
	}
	imageName, err := moduleDockerImageName(moduleRoot)
	if err != nil {
		return xerrors.Errorf("list builds: %w", err)
	}
	c := exec.CommandContext(ctx, "docker", "images", imageName)
	c.Stdout = pctx.stdout
	c.Stderr = pctx.stderr
	if err := c.Run(); err != nil {
		return xerrors.Errorf("list builds: %w", err)
	}
	return nil
}

func moduleDockerImageName(moduleRoot string) (string, error) {
	dockerfilePath := filepath.Join(moduleRoot, "Dockerfile")
	dockerfile, err := ioutil.ReadFile(dockerfilePath)
	if err != nil {
		return "", xerrors.Errorf("finding module Docker image name: %w", err)
	}
	imageName, err := parseImageNameFromDockerfile(dockerfile)
	if err != nil {
		return "", xerrors.Errorf("finding module Docker image name: parse %s: %w", dockerfilePath, err)
	}
	return imageName, nil
}

// parseImageNameFromDockerfile finds the magic "# gocdk-image:" comment in a
// Dockerfile and returns the image name.
func parseImageNameFromDockerfile(dockerfile []byte) (string, error) {
	const magic = "# gocdk-image:"
	commentStart := bytes.Index(dockerfile, []byte(magic))
	if commentStart == -1 {
		return "", xerrors.New("source does not contain the comment \"# gocdk-image:\"")
	}
	// TODO(light): Keep searching if comment does not start at beginning of line.
	nameStart := commentStart + len(magic)
	lenName := bytes.Index(dockerfile[nameStart:], []byte("\n"))
	if lenName == -1 {
		// No newline, go to end of file.
		lenName = len(dockerfile) - nameStart
	}
	name := string(dockerfile[nameStart : nameStart+lenName])
	return strings.TrimSpace(name), nil
}
