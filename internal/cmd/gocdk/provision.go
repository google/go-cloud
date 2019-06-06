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
	"errors"

	"github.com/spf13/cobra"
)

func registerProvisionCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	provisionCmd := &cobra.Command{
		Use:   "provision",
		Short: "TODO Provision resources",
		Long:  "TODO more about provisioning",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "TODO provision list ",
		Long:  "TODO more about provisioning",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			provisionList(pctx)
			return nil
		},
	}
	provisionCmd.AddCommand(listCmd)

	addCmd := &cobra.Command{
		Use:   "add BIOME_NAME TYPE",
		Short: "TODO provision add BIOME_NAME TYPE",
		Long:  "TODO more about provisioning",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return provisionAdd(ctx, pctx, args[0], args[1])
		},
	}
	provisionCmd.AddCommand(addCmd)

	rootCmd.AddCommand(provisionCmd)
}

// The "provision list" command.
func provisionList(pctx *processContext) {
	pctx.Logf("NOT YET IMPLEMENTED")
}

// The "provision add" command.
func provisionAdd(ctx context.Context, pctx *processContext, biome, typ string) error {
	return errors.New("not yet implemented")
}
