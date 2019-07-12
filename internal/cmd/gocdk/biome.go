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
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/prompt"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
	"golang.org/x/xerrors"
)

func registerBiomeCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	biomeCmd := &cobra.Command{
		Use:   "biome",
		Short: "TODO Manage biomes",
		Long:  "TODO more about biomes",
	}

	var launcher string
	biomeAddCmd := &cobra.Command{
		Use:   "add BIOME_NAME",
		Short: "TODO Add BIOME_NAME",
		Long:  "TODO more about adding biomes",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return biomeAdd(ctx, pctx, args[0], launcher)
		},
	}
	// TODO(rvangent): There should be a way to specify answer to launcher-specific
	// prompts via flags. Maybe a JSON snippet?
	biomeAddCmd.Flags().StringVar(&launcher, "launcher", "", "the launcher for the new biome")
	biomeCmd.AddCommand(biomeAddCmd)

	biomeListCmd := &cobra.Command{
		Use:   "list",
		Short: "TODO list",
		Long:  "TODO more about listing biomes",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return biomeList(ctx, pctx)
		},
	}
	biomeCmd.AddCommand(biomeListCmd)

	var input bool
	applyCmd := &cobra.Command{
		Use:   "apply BIOME_NAME",
		Short: "TODO Apply Terraform for BIOME",
		Long:  "TODO more about apply",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return biomeApply(ctx, pctx, args[0], input)
		},
	}
	applyCmd.Flags().BoolVar(&input, "input", true, "ask for input for Terraform variables if not directly set")
	biomeCmd.AddCommand(applyCmd)

	// TODO(rvangent): More biome subcommands: delete, list, ...?

	rootCmd.AddCommand(biomeCmd)
}

// launchSpecifier is used to pass launch specifier key/value pairs to
// outputs.tf for a new biome.
type launchSpecifier struct {
	Key, Value string
}

// launcherData is passed as template data to biome.json for a new biome.
type launcherData struct {
	Launcher     string
	ServeEnabled bool
}

// biomeAdd implements the "biome add" subcommand.
//
// It creates a new biome, prompting the user for the launcher to use if needed,
// as well as any launcher-specific parameters needed.
func biomeAdd(ctx context.Context, pctx *processContext, biome, launcher string) error {
	pctx.Logf("Adding biome %q...", biome)

	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("biome add: %w", err)
	}

	if launcher == "" {
		var err error
		// TODO(rvangent): Do this as a selection instead of freeform input?
		reader := bufio.NewReader(pctx.stdin)
		launcher, err = prompt.String(reader, pctx.stderr, "Please enter a launcher to use (local, cloudrun, ecs)", "local")
		if err != nil {
			return err
		}
	}

	data := &launcherData{Launcher: launcher}
	var launchSpecifiers []*launchSpecifier
	var saveActions []*static.Action
	switch launcher {
	case "local":
		data.ServeEnabled = true
		launchSpecifiers = append(launchSpecifiers, &launchSpecifier{Key: "host_port", Value: "8080"})
	case "cloudrun":
		pctx.Logf("")
		pctx.Logf("To launch on cloudrun, we need a few pieces of information.")
		reader := bufio.NewReader(pctx.stdin)
		projectID, err := prompt.GCPProjectID(reader, pctx.stderr)
		if err != nil {
			return xerrors.Errorf("biome add: %w", err)
		}
		saveActions = append(saveActions, static.AddLocal(prompt.GCPProjectIDTfLocalName, projectID))
		// TODO(rvangent): Eventually we should ask for the GCP region; currently
		// cloudrun is singly homed in us-central1.
		region := "us-central1"
		saveActions = append(saveActions, static.AddLocal(prompt.GCPRegionTfLocalName, region))
		serviceName, err := prompt.String(reader, pctx.stderr, "Please enter a service name", "myservice")
		if err != nil {
			return xerrors.Errorf("biome add: %w", err)
		}
		saveActions = append(saveActions,
			static.AddProvider("google"),
			&static.Action{
				SourcePath:  "/launchers/cloudrun.tf",
				DestRelPath: "main.tf",
				DestExists:  true,
			},
		)
		launchSpecifiers = append(launchSpecifiers,
			&launchSpecifier{Key: "project_id", Value: strconv.Quote("${local.gcp_project}")},
			&launchSpecifier{Key: "location", Value: strconv.Quote("${local.gcp_region}")},
			&launchSpecifier{Key: "service_name", Value: strconv.Quote(serviceName)},
		)
	case "ecs":
		pctx.Logf("")
		pctx.Logf("To launch on ECS, we need a few pieces of information.")
		reader := bufio.NewReader(pctx.stdin)
		region, err := prompt.AWSRegion(reader, pctx.stderr)
		if err != nil {
			return xerrors.Errorf("biome add: %w", err)
		}
		saveActions = append(saveActions,
			static.AddLocal(prompt.AWSRegionTfLocalName, region),
			static.AddProvider("aws"),
			static.AddProvider("random"),
			&static.Action{
				SourcePath:   "/launchers/ecs.tf",
				DestRelPath:  "ecs.tf",
				TemplateData: struct{ ProjectName string }{filepath.Base(moduleRoot)},
			},
		)
		launchSpecifiers = append(launchSpecifiers,
			&launchSpecifier{Key: "cluster", Value: "aws_ecs_cluster.default.name"},
			&launchSpecifier{Key: "region", Value: "local." + prompt.AWSRegionTfLocalName},
			&launchSpecifier{Key: "image_name", Value: "aws_ecr_repository.default.repository_url"},
		)
	default:
		return fmt.Errorf("biome add: %q is not a supported launcher", launcher)
	}

	// Create the new biome directory, copied from /biome in the static data.
	// Two files are treated as templates.
	actions, err := static.CopyDir("/biome")
	if err != nil {
		return xerrors.Errorf("biome add: %w", err)
	}
	for _, a := range actions {
		switch a.SourcePath {
		case "/biome/biome.json":
			a.TemplateData = data
		case "/biome/outputs.tf":
			a.TemplateData = launchSpecifiers
		}
	}
	actions = append(actions, saveActions...)
	biomePath, _ := biomeDir(moduleRoot, biome) // ignore expected "does not exist" error
	if err := static.Do(biomePath, nil, actions...); err != nil {
		return xerrors.Errorf("gocdk biome add: %w", err)
	}
	pctx.Logf("Success!")
	return nil
}

// biomeList implements the "biome list" subcommand.
//
// It lists the current set of biomes, computed as the subdirectories of the
// biome rootdir.
func biomeList(ctx context.Context, pctx *processContext) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("biome list: %w", err)
	}
	entries, err := ioutil.ReadDir(biomesRootDir(moduleRoot))
	if err != nil {
		return xerrors.Errorf("biome list: %w", err)
	}
	var biomes []string
	for _, entry := range entries {
		_, err := biomeDir(moduleRoot, entry.Name())
		if err == nil {
			biomes = append(biomes, entry.Name())
		}
	}
	sort.Strings(biomes)
	for _, biome := range biomes {
		pctx.Println(biome)
	}
	return nil
}

// biomeApply implements the "biome apply" subcommand.
//
// It runs "terraform init" and "terraform apply" for the named biome.
func biomeApply(ctx context.Context, pctx *processContext, biome string, input bool) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	}

	biomePath, err := biomeDir(moduleRoot, biome)
	if err != nil {
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	}

	c := pctx.NewCommand(ctx, biomePath, "terraform", "init", "-input="+strconv.FormatBool(input))
	if err := c.Run(); err != nil {
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	}

	// TODO(#1821): take over steps (plan, confirm, apply) so we can
	// dictate the messaging and errors. We should visually differentiate
	// when we insert verbiage on top of terraform.
	c = pctx.NewCommand(ctx, biomePath, "terraform", "apply", "-input="+strconv.FormatBool(input))
	if err := c.Run(); err != nil {
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	}
	return nil
}
