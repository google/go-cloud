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
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

// TODO(rvangent): This should probably not be in biome_add.go anymore.

func registerBiomeCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	biomeCmd := &cobra.Command{
		Use:   "biome",
		Short: "TODO Manage biomes",
		Long:  "TODO more about biomes",
	}
	biomeAddCmd := &cobra.Command{
		Use:   "add BIOME_NAME",
		Short: "TODO Add BIOME_NAME",
		Long:  "TODO more about adding biomes",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return biomeAdd(ctx, pctx, args[0])
		},
	}
	biomeCmd.AddCommand(biomeAddCmd)

	// TODO(rvangent): More biome subcommands.

	rootCmd.AddCommand(biomeCmd)
}

func biomeAdd(ctx context.Context, pctx *processContext, newName string) error {
	// TODO(clausti) interpolate launcher from one supplied as a flag
	projectDir, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		xerrors.Errorf("biome add: %w", err)
	}
	dstPath := findBiomeDir(projectDir, newName)
	data := struct {
		ProjectName string
	}{
		ProjectName: filepath.Base(projectDir),
	}

	if err := materializeTemplateDir(dstPath, "biome_add", data); err != nil {
		return xerrors.Errorf("gocdk biome add: %w", err)
	}
	fmt.Printf("Successfully added new biome '%v'!\n", newName)
	return nil
}
