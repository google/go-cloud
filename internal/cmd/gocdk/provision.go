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
	"path"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
	"golang.org/x/xerrors"
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
			return provisionList(pctx)
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

const provisionStaticRootDir = "/provision"

// availableTypes scans the static data to identify the available types for
// provision. Each returned name is of the form "<portable type>/<package>";
// for example, "blob/gcsblob". They are returned in sorted order.
func availableTypes(pctx *processContext) ([]string, error) {
	// Get the list of portable APIs, e.g., "blob".
	pts, err := static.List(provisionStaticRootDir, false)
	if err != nil {
		return nil, err
	}
	// The subdirectories under each portable API directory are the providers.
	var retval []string
	for _, pt := range pts {
		if !pt.IsDir() {
			continue
		}
		ptName := pt.Name()
		providers, err := static.List(path.Join(provisionStaticRootDir, ptName), false)
		if err != nil {
			return nil, err
		}
		for _, p := range providers {
			if !p.IsDir() {
				continue
			}
			retval = append(retval, path.Join(ptName, p.Name()))
		}
	}
	return retval, nil
}

// The "provision list" command.
func provisionList(pctx *processContext) error {
	avail, err := availableTypes(pctx)
	if err != nil {
		return xerrors.Errorf("provision list: %w", err)
	}
	for _, a := range avail {
		pctx.Println(a)
	}
	return nil
}

// The "provision add" command.
// TODO(rvangent): Can we support adding a particular type more than once?
// TODO(rvangent): If things fail in the middle, we are in an undefined state.
//                 Unclear how to handle that....
// TODO(rvangent): Modifying Terraform files in place means that we need to run
//                 "terraform init" again; currently we don't; see
//                 https://github.com/google/go-cloud/issues/2291.
// TODO(rvangent): Currently there are a bunch of default values for locals, so
//                 "terraform apply" works fine. Add the ability to prompt the user
//                 for things (e.g., GCP project ID), and use those values.
func provisionAdd(ctx context.Context, pctx *processContext, biome, typ string) error {
	pctx.Logf("Adding %q to %q...", typ, biome)

	moduleDir, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("provision add: %w", err)
	}

	// Make sure typ is valid.
	avail, err := availableTypes(pctx)
	if err != nil {
		return xerrors.Errorf("provision add: %w", err)
	}
	found := false
	for _, a := range avail {
		if a == typ {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("provision add: %q is not a supported type; use 'gocdk provision list' to see available types", typ)
	}

	srcPath := path.Join(provisionStaticRootDir, typ)
	dstPath := biomeDir(moduleDir, biome)
	if err := static.Materialize(dstPath, srcPath, &static.MaterializeOptions{Logger: pctx.errlog}); err != nil {
		return xerrors.Errorf("provision add: %w", err)
	}
	pctx.Logf("Success!")
	return nil
}
