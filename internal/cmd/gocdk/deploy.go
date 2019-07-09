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

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func registerDeployCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	deployCmd := &cobra.Command{
		Use:   "deploy BIOME",
		Short: "TODO Deploy the biome",
		Long:  "TODO more about deploy",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// Precompute snapshot tag and pass into build and launch.
			moduleRoot, err := pctx.ModuleRoot(ctx)
			if err != nil {
				return xerrors.Errorf("gocdk deploy: %w", err)
			}
			imageName, err := moduleDockerImageName(moduleRoot)
			if err != nil {
				return xerrors.Errorf("gocdk deploy: %w", err)
			}
			tag, err := generateTag()
			if err != nil {
				return xerrors.Errorf("gocdk deploy: %w", err)
			}
			snapshotRef := imageName + ":" + tag
			buildRefs := []string{
				imageName + defaultDockerTag,
				snapshotRef,
			}

			// Run build, "biome apply", and launch.
			biome := args[0]
			if err := build(ctx, pctx, buildRefs); err != nil {
				return err
			}
			if err := biomeApply(ctx, pctx, biome, true); err != nil {
				return err
			}
			if err := launch(ctx, pctx, biome, snapshotRef); err != nil {
				return err
			}
			return nil
		},
	}
	rootCmd.AddCommand(deployCmd)
}
