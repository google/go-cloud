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
	"strings"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
	"golang.org/x/xerrors"
)

// In sorted order.
var allDemos = []string{"blob", "docstore", "pubsub", "runtimevar", "secrets"}

func registerDemoCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {

	demoCmd := &cobra.Command{
		Use:   "demo",
		Short: "Manage Go CDK portable type demos",
		Long: `Demos consist of source code added to your application that demonstrate
the functionality of a particular Go CDK portable type.

By default, each demo uses a local implementation of the portable type; for
example, the "blob" demo uses an in-memory implementation of "blob".

You can use the "gocdk resource" command to add a different resource to a
biome; for example, "gocdk resource add blob/gcsblob" will provision a Google Compute
Storage bucket, and the "blob" demo will work it instead (with no code changes!).`,
	}

	demoListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available portable type demos",
		Long:  `Print the list of available portable type demos.`,
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return listDemos(pctx)
		},
	}
	demoCmd.AddCommand(demoListCmd)

	var force bool
	demoAddCmd := &cobra.Command{
		Use:   "add <demo name>",
		Short: "Add a portable type demo",
		Long: `Add Go source code that demonstrates the functionality of a particular
Go CDK portable type.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return addDemo(ctx, pctx, args[0], force)
		},
	}
	demoAddCmd.Flags().BoolVar(&force, "force", false, "re-add the demo source file even if it has already been added, overwriting any previous file")
	demoCmd.AddCommand(demoAddCmd)

	rootCmd.AddCommand(demoCmd)
}

func listDemos(pctx *processContext) error {
	pctx.Println(strings.Join(allDemos, "\n"))
	return nil
}

func addDemo(ctx context.Context, pctx *processContext, demoToAdd string, force bool) error {
	moduleRoot, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("demo add: %w", err)
	}
	for _, demo := range allDemos {
		if demo == demoToAdd {
			return instantiateDemo(pctx, moduleRoot, demo, force)
		}
	}
	return xerrors.Errorf("%q is not a supported demo; try 'gocdk demo list' to see available demos", demoToAdd)
}

// instantiateDemo does all of the work required to add a demo of a
// portable API to the user's project.
func instantiateDemo(pctx *processContext, moduleRoot, demo string, force bool) error {
	pctx.Logf("Adding %q...", demo)

	// Copy a single file, holding the Go source code for the demo, into
	// the root of the user's project.
	filename := fmt.Sprintf("demo_%s.go", demo)
	action := static.CopyFile(path.Join("/demo", filename), filename)

	opts := &static.Options{Force: force, Logger: pctx.errlog}
	if err := static.Do(moduleRoot, opts, action); err != nil {
		return err
	}
	pctx.Logf("Run 'gocdk serve' and visit http://localhost:8080/demo/%s to see the demo.", demo)
	return nil
}
