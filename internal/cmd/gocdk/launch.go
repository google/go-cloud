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
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/spf13/cobra"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/cmd/gocdk/internal/docker"
	"gocloud.dev/internal/cmd/gocdk/internal/launcher"
	"golang.org/x/xerrors"
	"google.golang.org/api/option"
	cloudrun "google.golang.org/api/run/v1alpha1"
)

func registerLaunchCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	var dockerImage string
	launchCmd := &cobra.Command{
		Use:   "launch BIOME",
		Short: "TODO Launch BIOME",
		Long:  "TODO more about launch",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return launch(ctx, pctx, args[0], dockerImage)
		},
	}
	launchCmd.Flags().StringVar(&dockerImage, "image", defaultDockerTag, "Docker image to launch in the form `name[:tag] OR :tag`")
	rootCmd.AddCommand(launchCmd)
}

func launch(ctx context.Context, pctx *processContext, biome, dockerImage string) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Get the image name from the Dockerfile if not specified.
	if dockerImage == "" || strings.HasPrefix(dockerImage, ":") {
		name, err := moduleDockerImageName(moduleRoot)
		if err != nil {
			return xerrors.Errorf("gocdk launch: %w", err)
		}
		dockerImage = name + dockerImage
	}

	biomePath, err := biomeDir(moduleRoot, biome)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Prepare the launcher.
	cfg, err := readBiomeConfig(moduleRoot, biome)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}
	if cfg.Launcher == nil {
		return xerrors.Errorf("gocdk launch: launcher not specified in %s", filepath.Join(biomePath, biomeConfigFileName))
	}
	myLauncher, err := newLauncher(ctx, pctx, *cfg.Launcher)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Read the launch specifier from the biome's Terraform output.
	tfOutput, err := tfReadOutput(ctx, biomePath, pctx.env)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}
	env, err := launchEnv(tfOutput)
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}

	// Launch the application.
	launchURL, err := myLauncher.Launch(ctx, &launcher.Input{
		DockerImage: dockerImage,
		Env:         env,
		Specifier:   tfOutput["launch_specifier"].mapValue(),
	})
	if err != nil {
		return xerrors.Errorf("gocdk launch: %w", err)
	}
	pctx.Logf("Serving at %s\n", launchURL)
	return nil
}

// Launcher is the interface for any type that can launch a Docker image.
type Launcher interface {
	Launch(ctx context.Context, input *launcher.Input) (*url.URL, error)
}

// newLauncher creates the launcher for the given name.
func newLauncher(ctx context.Context, pctx *processContext, launcherName string) (Launcher, error) {
	switch launcherName {
	case "local":
		return &launcher.Local{
			Logger:       pctx.errlog,
			DockerClient: docker.New(pctx.env),
		}, nil
	case "cloudrun":
		creds, err := pctx.gcpCredentials(ctx)
		if err != nil {
			return nil, xerrors.Errorf("prepare cloudrun launcher: %w", err)
		}
		httpClient, _ := gcp.NewHTTPClient(http.DefaultTransport, creds.TokenSource)
		runService, err := cloudrun.NewService(ctx, option.WithHTTPClient(&httpClient.Client))
		if err != nil {
			return nil, xerrors.Errorf("prepare cloudrun launcher: %w", err)
		}
		return &launcher.CloudRun{
			Logger:       pctx.errlog,
			Client:       runService,
			DockerClient: docker.New(pctx.env),
		}, nil
	case "ecs":
		sess, err := session.NewSession()
		if err != nil {
			return nil, xerrors.Errorf("prepare ecs launcher: %w", err)
		}
		return &launcher.ECS{
			Logger:         pctx.errlog,
			ConfigProvider: sess,
			DockerClient:   docker.New(pctx.env),
		}, nil
	default:
		return nil, xerrors.Errorf("prepare launcher: unknown launcher %q", launcherName)
	}
}
