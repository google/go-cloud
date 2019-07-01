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
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/docker"
	"golang.org/x/xerrors"
)

const defaultDockerTag = ":latest"

func registerBuildCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	var list bool
	var ref string
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "TODO Build a Docker image",
		Long:  "TODO more about build",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if list {
				if err := listBuilds(ctx, pctx); err != nil {
					return xerrors.Errorf("gocdk build: %w", err)
				}
				return nil
			}
			return build(ctx, pctx, ref)
		},
	}
	buildCmd.Flags().BoolVar(&list, "list", false, "display Docker images of this project")
	buildCmd.Flags().StringVarP(&ref, "tag", "t", defaultDockerTag, "name and/or tag in the form `name[:tag] OR :tag`")
	rootCmd.AddCommand(buildCmd)
}

// TODO(rvangent): Rename ref and/or dockerTag for consistency?
// https://github.com/google/go-cloud/pull/2144#discussion_r288625539
func build(ctx context.Context, pctx *processContext, ref string) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("gocdk build: %w", err)
	}
	if strings.HasPrefix(ref, ":") {
		imageName, err := moduleDockerImageName(moduleRoot)
		if err != nil {
			return xerrors.Errorf("gocdk build: %w", err)
		}
		ref = imageName + ref
	}
	if err := docker.New(pctx.env).Build(ctx, ref, moduleRoot, pctx.stderr); err != nil {
		return xerrors.Errorf("gocdk build: %w", err)
	}
	return nil
}

func listBuilds(ctx context.Context, pctx *processContext) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("list builds: %w", err)
	}
	imageName, err := moduleDockerImageName(moduleRoot)
	if err != nil {
		return xerrors.Errorf("list builds: %w", err)
	}
	images, err := docker.New(pctx.env).ListImages(ctx, imageName)
	if err != nil {
		return xerrors.Errorf("list builds: %w", err)
	}
	for _, image := range images {
		if image.Repository == "" || image.Tag == "" {
			pctx.Printf("@%-60s  %s\n", image.Digest, image.CreatedAt.Local().Format(time.Stamp))
		} else {
			pctx.Printf("%-60s  %s\n", image.Repository+":"+image.Tag, image.CreatedAt.Local().Format(time.Stamp))
		}
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
	if _, tag, digest := docker.ParseImageRef(name); tag != "" || digest != "" {
		return "", xerrors.Errorf("image name %q must not contain a tag or digest")
	}
	return strings.TrimSpace(name), nil
}
