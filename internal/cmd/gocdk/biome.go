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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/prompt"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
	"golang.org/x/xerrors"
)

func registerBiomeCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	biomeCmd := &cobra.Command{
		Use:   "biome",
		Short: "Manage biomes",
		Long: `Biomes are target environments. Each biome has a distinct deployment target,
and maintains its own set of Cloud resources.

A typical set of biomes is "dev", "staging", "prod".`,
	}

	var launcher string
	addCmd := &cobra.Command{
		Use:   "add <biome name>",
		Short: "Add a new biome",
		Long: `Add a new biome. Unless you specify answers via flags, you will be prompted
to choose a deployment target for the biome, as well as for any deployment-specific
information needed to deploy.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return biomeAdd(ctx, pctx, args[0], launcher)
		},
	}
	// TODO(rvangent): There should be a way to specify answer to launcher-specific
	// prompts via flags. Maybe a JSON snippet?
	addCmd.Flags().StringVar(&launcher, "launcher", "", "the launcher for the new biome")
	biomeCmd.AddCommand(addCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Print the list of existing biomes",
		Long:  `Print the list of existing biomes.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return biomeList(ctx, pctx)
		},
	}
	biomeCmd.AddCommand(listCmd)

	var applyOpts biomeApplyOptions
	applyCmd := &cobra.Command{
		Use:   "apply <biome name>",
		Short: "Apply any changes required for the biome's resource configuration (e.g., creating Cloud resources)",
		Long: `Apply the changes required to reach the desired state of the biome's resource
configuration (e.g., creating or updating Cloud resources).

Runs "terraform init" followed by "terraform apply".`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return biomeApply(ctx, pctx, args[0], &applyOpts)
		},
	}
	applyCmd.Flags().BoolVar(&applyOpts.Input, "input", true, "ask for input for Terraform variables if not directly set")
	applyCmd.Flags().BoolVar(&applyOpts.Verbose, "verbose", false, "true to print all output from Terraform")
	applyCmd.Flags().BoolVar(&applyOpts.Quiet, "quiet", false, "true to suppress all output from Terraform unless there's an error")
	applyCmd.Flags().BoolVar(&applyOpts.AutoApprove, "auto-approve", false, "true to auto-approve resource changes")
	biomeCmd.AddCommand(applyCmd)

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
				SourcePath:  "/launchers/ecs.tf",
				DestRelPath: "ecs.tf",
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

// biomeApply holds options for "biome apply".
type biomeApplyOptions struct {
	Quiet       bool // suppress all Terraform output unless there's an error
	Verbose     bool // show all Terraform output
	AutoApprove bool // auto-approve Terraform changes
	Input       bool // allow Terraform prompts for input
}

// biomeApply implements the "biome apply" subcommand.
//
// It runs "terraform init", "terraform plan", and "terraform apply" for the
// named biome.
func biomeApply(ctx context.Context, pctx *processContext, biome string, opts *biomeApplyOptions) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	}

	biomePath, err := biomeDir(moduleRoot, biome)
	if err != nil {
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	}

	writeTerraformOutput := func(cmd *exec.Cmd, msg []byte) {
		const separator = "#########################################\n"
		pctx.stderr.Write([]byte("\n"))
		pctx.stderr.Write([]byte(separator))
		pctx.stderr.Write([]byte(fmt.Sprintf("Output from %q\n", strings.Join(cmd.Args, " "))))
		pctx.stderr.Write([]byte(separator))
		pctx.stderr.Write(msg)
		pctx.stderr.Write([]byte(separator))
		pctx.stderr.Write([]byte("\n"))
	}

	pctx.Logf("Checking to see if resource updates are needed...")

	// First, run "terraform init". It's needed the first time, and anytime
	// a new Terraform provider is added, so it's safest to just always run it.
	inputArg := fmt.Sprintf("-input=%v", opts.Input)
	c := pctx.NewCommand(ctx, biomePath, "terraform", "init", inputArg)
	c.Env = overrideEnv(c.Env, "TF_IN_AUTOMATION=1")
	c.Stdout = nil
	c.Stderr = nil
	if out, err := c.CombinedOutput(); err != nil {
		writeTerraformOutput(c, out)
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	} else if opts.Verbose {
		writeTerraformOutput(c, out)
	}

	// Next, run "terraform plan".
	planFile := filepath.Join(biomePath, "tf.plan")
	defer os.Remove(planFile)
	c = pctx.NewCommand(ctx, biomePath, "terraform", "plan", "-detailed-exitcode", "-out="+planFile, inputArg)
	c.Env = overrideEnv(c.Env, "TF_IN_AUTOMATION=1")
	c.Stdout = nil
	c.Stderr = nil

	// No error means success and no diffs.
	// Exit code 1 means the command failed.
	// Exit code 2 means the command succeeded, but there's a diff.
	if out, err := c.CombinedOutput(); err != nil && !strings.Contains(err.Error(), "exit status 2") {
		// A real failure.
		writeTerraformOutput(c, out)
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	} else if err != nil {
		// Exit code 2 --> there's a diff.
		// Print them out and prompt for approval.
		pctx.Logf("Resources updates are needed.")
		if opts.Quiet {
			pctx.Logf("Not showing update details due to --quiet.")
		} else {
			writeTerraformOutput(c, out)
		}
		if !opts.AutoApprove {
			reader := bufio.NewReader(pctx.stdin)
			if approve, err := prompt.String(reader, pctx.stderr, "Do you want to perform these actions (only 'yes' will be accepted to approve)?", ""); err != nil {
				return xerrors.Errorf("biome apply %s: %w", biome, err)
			} else if approve != "yes" {
				return errors.New("cancelled")
			}
		}
	} else {
		// No changes.
		pctx.Logf("No resources updates are needed.")
		if opts.Verbose {
			writeTerraformOutput(c, out)
		}
	}

	// Finally, run "terraform apply" on the plan.
	// Note: do this even if there are no resource changes, because the
	// plan can include changes to other things, like output variables.
	// For example:
	// 1. Edit "outputs.tf" and change an output variable value.
	// 2. "terraform output" still prints the old value.
	// 3. "terraform plan -out tf.plan" returns 0 and says "no changes", but tf.plan is non-empty.
	// 4. "terraform output" still prints the old value.
	// 5. "terraform apply tf.plan"...
	// 6. "terraform ouutput" now prints the new value.
	// See https://github.com/hashicorp/terraform/issues/15419.
	c = pctx.NewCommand(ctx, biomePath, "terraform", "apply", planFile)
	c.Env = overrideEnv(c.Env, "TF_IN_AUTOMATION=1")
	c.Stdout = nil
	c.Stderr = nil
	if out, err := c.CombinedOutput(); err != nil {
		writeTerraformOutput(c, out)
		return xerrors.Errorf("biome apply %s: %w", biome, err)
	} else if opts.Verbose {
		writeTerraformOutput(c, out)
	}
	pctx.Logf("Success!")
	return nil
}
