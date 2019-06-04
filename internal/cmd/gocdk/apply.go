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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func registerApplyCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	var input bool
	applyCmd := &cobra.Command{
		Use:   "apply BIOME",
		Short: "TODO Apply Terraform for BIOME",
		Long:  "TODO more about apply",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return apply(ctx, pctx, args[0], input)
		},
	}
	applyCmd.Flags().BoolVar(&input, "input", true, "ask for input for Terraform variables if not directly set")
	rootCmd.AddCommand(applyCmd)
}

func apply(ctx context.Context, pctx *processContext, biome string, input bool) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("apply %s: %w", biome, err)
	}

	if err := ensureTerraformInit(ctx, pctx, moduleRoot, biome, input); err != nil {
		return xerrors.Errorf("apply %s: %w", biome, err)
	}

	// TODO(#1821): take over steps (plan, confirm, apply) so we can
	// dictate the messaging and errors. We should visually differentiate
	// when we insert verbiage on top of terraform.
	c := exec.CommandContext(ctx, "terraform", "apply", "-input="+strconv.FormatBool(input))
	c.Dir = biomeDir(moduleRoot, biome)
	c.Env = pctx.env
	c.Stdin = pctx.stdin
	c.Stdout = pctx.stdout
	c.Stderr = pctx.stderr
	if err := c.Run(); err != nil {
		return xerrors.Errorf("apply %s: %w", biome, err)
	}
	return nil
}

// ensureTerraformInit checks for a .terraform directory at the biome root.
// If one doesn't exist, ensureTerraformInit runs terraform init.
func ensureTerraformInit(ctx context.Context, pctx *processContext, moduleRoot, biome string, input bool) error {
	// Check for .terraform directory.
	biomePath := biomeDir(moduleRoot, biome)
	_, err := os.Stat(filepath.Join(biomePath, ".terraform"))
	if err == nil {
		// .terraform exists, no op.
		return nil
	}
	if !os.IsNotExist(err) {
		// Some other error occurred.
		return xerrors.Errorf("ensure terraform init: %w", err)
	}

	// .terraform directory does not exist; make sure biome directory exists.
	_, err = os.Stat(biomePath)
	if os.IsNotExist(err) {
		notFound := &biomeNotFoundError{
			moduleRoot: moduleRoot,
			biome:      biome,
			frame:      xerrors.Caller(0),
			detail:     err,
		}
		return xerrors.Errorf("ensure terraform init: %w", notFound)
	}
	if err != nil {
		return xerrors.Errorf("ensure terraform init: %w", err)
	}

	// Biome exists but not initialized. Need to run terraform init.
	c := exec.CommandContext(ctx, "terraform", "init", "-input="+strconv.FormatBool(input))
	c.Dir = biomePath
	c.Env = pctx.env
	c.Stdin = pctx.stdin
	c.Stdout = pctx.stdout
	c.Stderr = pctx.stderr
	if err := c.Run(); err != nil {
		return xerrors.Errorf("ensure terraform init: %w", err)
	}
	return nil
}
