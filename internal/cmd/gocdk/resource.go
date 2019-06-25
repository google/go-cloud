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
	"sort"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/prompt"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
	"golang.org/x/xerrors"
)

func registerResourceCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	resourceCmd := &cobra.Command{
		Use:   "resource",
		Short: "TODO Resources",
		Long:  "TODO more about provisioning resources",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "TODO resource list ",
		Long:  "TODO more about provisioning resources",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return resourceList(pctx)
		},
	}
	resourceCmd.AddCommand(listCmd)

	addCmd := &cobra.Command{
		Use:   "add BIOME_NAME TYPE",
		Short: "TODO resource add BIOME_NAME TYPE",
		Long:  "TODO more about provisioning resources",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return resourceAdd(ctx, pctx, args[0], args[1])
		},
	}
	resourceCmd.AddCommand(addCmd)

	rootCmd.AddCommand(resourceCmd)
}

var provisionableTypes = map[string]func(*processContext, string) ([]*static.Action, error){

	// BLOB
	"blob/azureblob": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		reader := bufio.NewReader(pctx.stdin)
		_, err := prompt.AzureLocationIfNeeded(reader, pctx.stderr, biomeDir)
		if err != nil {
			return nil, err
		}
		return []*static.Action{
			static.AddProvider("azurerm"),
			static.AddProvider("random"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.azureblob_bucket_url}"),
			static.AddOutputVar("AZURE_STORAGE_ACCOUNT", "${azurerm_storage_account.storage_account.name}"),
			static.AddOutputVar("AZURE_STORAGE_KEY", "${azurerm_storage_account.storage_account.primary_access_key}"),
			static.CopyFile("/resource/blob/azureblob.tf", "azureblob.tf"),
		}, nil
	},
	"blob/fileblob": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		return []*static.Action{
			static.AddProvider("local"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.fileblob_bucket_url}"),
			static.CopyFile("/resource/blob/fileblob.tf", "fileblob.tf"),
		}, nil
	},
	"blob/gcsblob": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		reader := bufio.NewReader(pctx.stdin)
		_, err := prompt.GCPProjectIDIfNeeded(reader, pctx.stderr, biomeDir)
		if err != nil {
			return nil, err
		}
		_, err = prompt.GCPStorageLocationIfNeeded(reader, pctx.stderr, biomeDir)
		if err != nil {
			return nil, err
		}
		return []*static.Action{
			static.AddProvider("google"),
			static.AddProvider("random"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.gcsblob_bucket_url}"),
			static.CopyFile("/resource/blob/gcsblob.tf", "gcsblob.tf"),
		}, nil
	},
	"blob/s3blob": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		reader := bufio.NewReader(pctx.stdin)
		_, err := prompt.AWSRegionIfNeeded(reader, pctx.stderr, biomeDir)
		if err != nil {
			return nil, err
		}
		return []*static.Action{
			static.AddProvider("aws"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.s3blob_bucket_url}"),
			static.CopyFile("/resource/blob/s3blob.tf", "s3blob.tf"),
		}, nil
	},

	// RUNTIMEVAR
	"runtimevar/awsparamstore": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		reader := bufio.NewReader(pctx.stdin)
		_, err := prompt.AWSRegionIfNeeded(reader, pctx.stderr, biomeDir)
		if err != nil {
			return nil, err
		}
		return []*static.Action{
			static.AddProvider("aws"),
			static.AddProvider("random"),
			static.AddOutputVar("RUNTIMEVAR_VARIABLE_URL", "${local.awsparamstore_url}"),
			static.CopyFile("/resource/runtimevar/awsparamstore.tf", "awsparamstore.tf"),
		}, nil
	},
	"runtimevar/filevar": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		return []*static.Action{
			static.AddProvider("local"),
			static.AddOutputVar("RUNTIMEVAR_VARIABLE_URL", "${local.filevar_url}"),
			static.CopyFile("/resource/runtimevar/filevar.tf", "filevar.tf"),
		}, nil
	},
	"runtimevar/gcpruntimeconfig": func(pctx *processContext, biomeDir string) ([]*static.Action, error) {
		reader := bufio.NewReader(pctx.stdin)
		_, err := prompt.GCPProjectIDIfNeeded(reader, pctx.stderr, biomeDir)
		if err != nil {
			return nil, err
		}
		return []*static.Action{
			static.AddProvider("google"),
			static.AddProvider("random"),
			static.AddOutputVar("RUNTIMEVAR_VARIABLE_URL", "${local.gcpruntimeconfig_url}"),
			static.CopyFile("/resource/runtimevar/gcpruntimeconfig.tf", "gcpruntimeconfig.tf"),
		}, nil
	},
}

// The "resource list" command.
func resourceList(pctx *processContext) error {
	var sorted []string
	for key := range provisionableTypes {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	for _, a := range sorted {
		pctx.Println(a)
	}
	return nil
}

// The "resource add" command.
// TODO(rvangent): Can we support adding a particular type more than once?
// TODO(rvangent): If things fail in the middle, we are in an undefined state.
//                 Unclear how to handle that....
// TODO(rvangent): Modifying Terraform files in place means that we need to run
//                 "terraform init" again; currently we don't; see
//                 https://github.com/google/go-cloud/issues/2291.
func resourceAdd(ctx context.Context, pctx *processContext, biome, typ string) error {
	pctx.Logf("Adding %q to %q...", typ, biome)

	moduleDir, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("resource add: %w", err)
	}
	destBiomeDir := biomeDir(moduleDir, biome)

	do := provisionableTypes[typ]
	if do == nil {
		return fmt.Errorf("resource add: %q is not a supported type; use 'gocdk resource list' to see available types", typ)
	}

	// Do the prompts for the chosen type.
	actions, err := do(pctx, destBiomeDir)
	if err != nil {
		return xerrors.Errorf("resource add: %w", err)
	}
	// Perform the actions for the chosen type, instantiating into the
	// chosen biome directory.
	opts := &static.Options{Logger: pctx.errlog}
	if err := static.Do(destBiomeDir, opts, actions...); err != nil {
		return xerrors.Errorf("resource add: %w", err)
	}
	pctx.Logf("Success!")
	return nil
}
