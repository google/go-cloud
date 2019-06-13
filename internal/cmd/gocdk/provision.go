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
	"sort"

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

var (
	provisionableTypes = map[string][]*static.Action{
		"blob/azureblob": {
			static.AddProvider("azurerm"),
			static.AddProvider("random"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.azureblob_bucket_url}"),
			static.AddOutputVar("AZURE_STORAGE_ACCOUNT", "${azurerm_storage_account.storage_account.name}"),
			static.AddOutputVar("AZURE_STORAGE_KEY", "${azurerm_storage_account.storage_account.primary_access_key}"),
			static.CopyFile("/provision/blob/azureblob.tf", "azureblob.tf"),
			// TODO(rvangent): Prompt for this if needed.
			static.AddLocal("azure_location", "westus"),
		},
		"blob/fileblob": {
			static.AddProvider("local"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.fileblob_bucket_url}"),
			static.CopyFile("/provision/blob/fileblob.tf", "fileblob.tf"),
		},
		"blob/gcsblob": {
			static.AddProvider("google"),
			static.AddProvider("random"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.gcsblob_bucket_url}"),
			static.CopyFile("/provision/blob/gcsblob.tf", "gcsblob.tf"),
			// TODO(rvangent): Prompt for these if needed.
			static.AddLocal("gcp_project", "go-cloud-test-216917"),
			static.AddLocal("gcs_bucket_location", "US"),
		},
		"blob/s3blob": {
			static.AddProvider("aws"),
			static.AddOutputVar("BLOB_BUCKET_URL", "${local.s3blob_bucket_url}"),
			static.CopyFile("/provision/blob/s3blob.tf", "s3blob.tf"),
			// TODO(rvangent): Prompt for this if needed.
			static.AddLocal("aws_region", "us-west-1"),
		},
	}
)

// The "provision list" command.
func provisionList(pctx *processContext) error {
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

// The "provision add" command.
// TODO(rvangent): Can we support adding a particular type more than once?
// TODO(rvangent): If things fail in the middle, we are in an undefined state.
//                 Unclear how to handle that....
// TODO(rvangent): Modifying Terraform files in place means that we need to run
//                 "terraform init" again; currently we don't; see
//                 https://github.com/google/go-cloud/issues/2291.
func provisionAdd(ctx context.Context, pctx *processContext, biome, typ string) error {
	pctx.Logf("Adding %q to %q...", typ, biome)

	moduleDir, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("provision add: %w", err)
	}

	actions := provisionableTypes[typ]
	if actions == nil {
		return fmt.Errorf("provision add: %q is not a supported type; use 'gocdk provision list' to see available types", typ)
	}
	// Perform the actions for the chosen type, instantiating into the
	// chosen biome directory.
	opts := &static.Options{Logger: pctx.errlog}
	if err := static.Do(biomeDir(moduleDir, biome), opts, actions...); err != nil {
		return xerrors.Errorf("provision add: %w", err)
	}
	pctx.Logf("Success!")
	return nil
}
