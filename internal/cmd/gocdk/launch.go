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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"

	"golang.org/x/xerrors"
)

func launch(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "launch")
	dockerImage := f.String("image", "", "Docker image to launch in the form `name[:tag]` (defaults to project latest)")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk launch: %w", err)
	}
	if f.NArg() != 1 {
		return usagef("gocdk launch BIOME")
	}
	biome := f.Arg(0)

	moduleRoot, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Get the image name from the Dockerfile if not specified.
	if *dockerImage == "" {
		var err error
		*dockerImage, err = moduleDockerImageName(moduleRoot)
		if err != nil {
			return xerrors.Errorf("gocdk launch: %w", err)
		}
	}

	// Prepare the launcher.
	cfg, err := readBiomeConfig(moduleRoot, biome)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}
	if cfg.Launcher == nil {
		return xerrors.Errorf("gocdk launch: launcher not specified in %s", filepath.Join(findBiomeDir(moduleRoot, biome), biomeConfigFileName))
	}
	launcher, err := newLauncher(ctx, pctx, *cfg.Launcher)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Read the launch specifier from the biome's Terraform output.
	tfOutput, err := tfReadOutput(ctx,
		findBiomeDir(moduleRoot, biome),
		pctx.env)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Launch the application.
	launchURL, err := launcher.Launch(ctx, &LaunchInput{
		DockerImage: *dockerImage,
		Specifier:   LaunchSpecifier(tfOutput["launch_specifier"].mapValue()),
		// TODO(light): Pass environment variables from tfOutput into env.
	})
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}
	fmt.Fprintf(pctx.stdout, "Serving at %s\n", launchURL)
	return nil
}

// TODO(light): Move Launcher and supporting types to their own package.

// Launcher is the interface for any type that can launch a Docker image.
type Launcher interface {
	Launch(ctx context.Context, input *LaunchInput) (*url.URL, error)
}

// LaunchInput is the input to a launcher.
type LaunchInput struct {
	// DockerImage specifies the image name and tag of the local Docker image to
	// deploy. If the local image does not exist, then the launcher should return
	// an error.
	DockerImage string

	// env is the set of additional environment variables to set. It should not
	// include PORT nor should it contain multiple entries for the same variable
	// name.
	Env []string

	// specifier is the set of arguments passed from a biome's Terraform module.
	Specifier LaunchSpecifier
}

// newLauncher creates the launcher for the given name.
func newLauncher(ctx context.Context, pctx *processContext, launcherName string) (Launcher, error) {
	logger := log.New(pctx.stderr, "gocdk: ", log.Ldate|log.Ltime)
	switch launcherName {
	case "local":
		return &localLauncher{
			logger:    logger,
			dockerEnv: pctx.env,
			dockerDir: pctx.workdir,
		}, nil
	default:
		return nil, xerrors.Errorf("prepare launcher: unknown launcher %q", launcherName)
	}
}

// localLauncher starts local Docker containers.
type localLauncher struct {
	logger    *log.Logger
	dockerEnv []string
	dockerDir string
}

// Launch implements Launcher.Launch.
func (local *localLauncher) Launch(ctx context.Context, input *LaunchInput) (*url.URL, error) {
	hostPort := input.Specifier.IntValue("host_port")
	if hostPort == 0 {
		hostPort = 8080
	} else if hostPort < 0 || hostPort > 65535 {
		return nil, xerrors.Errorf("local launch: host_port is out of range [0, 65535]")
	}
	dockerArgs := []string{
		"run",
		"--rm",
		"--detach",
		"--publish", fmt.Sprintf("%d:8080", hostPort),
	}
	for _, v := range input.Env {
		dockerArgs = append(dockerArgs, "--env", v)
	}
	dockerArgs = append(dockerArgs, "--env", "PORT=8080")
	dockerArgs = append(dockerArgs, input.DockerImage)

	c := exec.CommandContext(ctx, "docker", dockerArgs...)
	c.Env = local.dockerEnv
	c.Dir = local.dockerDir
	out, err := c.CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			return nil, xerrors.Errorf("local launch: docker run: %w", err)
		}
		return nil, xerrors.Errorf("local launch: docker run:\n%s", out)
	}

	containerID := string(bytes.TrimSuffix(out, []byte("\n")))
	local.logger.Printf("Docker container %s started, waiting for healthy...", containerID)
	serveURL := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", hostPort),
		Path:   "/",
	}
	healthCheckURL := &url.URL{
		Scheme: serveURL.Scheme,
		Host:   serveURL.Host,
		Path:   "/healthz/readiness",
	}
	if err := waitForHealthy(ctx, healthCheckURL); err != nil {
		// TODO(light): Run `docker stop`.
		return nil, xerrors.Errorf("local launch: %w", err)
	}
	local.logger.Printf("Container healthy! To shut down, run: docker stop %s", containerID)
	return serveURL, nil
}

// LaunchSpecifier is the set of arguments passed from a biome's Terraform
// module to the biome's launcher.
type LaunchSpecifier map[string]interface{}

// StringValue returns the specifier's value for a key if it is a string.
func (spec LaunchSpecifier) StringValue(key string) string {
	v, _ := spec[key].(string)
	return v
}

// IntValue returns the specifier's value for a key if it is an integer.
func (spec LaunchSpecifier) IntValue(key string) int {
	switch v := spec[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		i, _ := strconv.ParseInt(v, 10, 0)
		return int(i)
	default:
		return 0
	}
}
