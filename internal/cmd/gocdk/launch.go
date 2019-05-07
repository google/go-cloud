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
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"
)

func launch(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "launch")
	imageName := f.String("image", "", "Docker image name to launch (defaults to project latest)")
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
	if *imageName == "" {
		var err error
		*imageName, err = moduleDockerImageName(moduleRoot)
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
		pctx.overrideEnv("TF_IN_AUTOMATION=1"))
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Launch the application.
	launchURL, err := launcher.Launch(ctx, &launchInput{
		dockerImage: *imageName,
		specifier:   launchSpecifier(tfOutput["launch_specifier"].mapValue()),
		// TODO(light): Pass environment variables from tfOutput into env.
	})
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}
	fmt.Fprintf(pctx.stdout, "Serving at %s\n", launchURL)
	return nil
}

// Launcher is the interface for any type that can launch a Docker image.
type Launcher interface {
	Launch(ctx context.Context, input *launchInput) (*url.URL, error)
}

// launchInput is the input to a launcher.
type launchInput struct {
	// dockerImage specifies the image name and tag of the local Docker image to
	// deploy. If the local image does not exist, then the launcher should return
	// an error.
	dockerImage string

	// env is the set of additional environment variables to set. It should not
	// include PORT nor should it contain multiple entries for the same variable
	// name.
	env []string

	// specifier is the set of arguments passed from a biome's Terraform module.
	specifier launchSpecifier
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
func (local *localLauncher) Launch(ctx context.Context, input *launchInput) (*url.URL, error) {
	dockerArgs := []string{
		"run",
		"--rm",
		"--detach",
		"--publish", "8080:8080",
	}
	for _, v := range input.env {
		dockerArgs = append(dockerArgs, "--env", v)
	}
	dockerArgs = append(dockerArgs, "--env", "PORT=8080")
	dockerArgs = append(dockerArgs, input.dockerImage)

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

	local.logger.Printf("Docker container %s started, waiting for healthy...", bytes.TrimSuffix(out, []byte("\n")))
	serveURL := &url.URL{
		Scheme: "http",
		Host:   "localhost:8080",
		Path:   "/",
	}
	healthCheckURL := &url.URL{
		Scheme: serveURL.Scheme,
		Host:   serveURL.Host,
		Path:   "/healthz/readiness",
	}
	if err := waitForHealthy(ctx, healthCheckURL); err != nil {
		return nil, xerrors.Errorf("local launch: %w", err)
	}
	return serveURL, nil
}

// launchSpecifier is the set of arguments passed from a biome's Terraform
// module to the biome's launcher.
type launchSpecifier map[string]interface{}

// stringValue returns the specifier's value if it is a string.
func (spec launchSpecifier) stringValue(key string) string {
	v, _ := spec[key].(string)
	return v
}
