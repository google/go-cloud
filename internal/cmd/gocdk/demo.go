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
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

type demoInfo struct {
	name       string // the portable API name
	goDemoPath string // the path to the .go file to add to the project for static.Open
	demoURL    string // the URL for the demo
}

var allDemos = []*demoInfo{
	{
		name:       "blob",
		goDemoPath: "/demo/blob/demo_blob.go",
		demoURL:    "/demo/blob/",
	},
	{
		name:       "pubsub",
		goDemoPath: "/demo/pubsub/demo_pubsub.go",
		demoURL:    "/demo/pubsub/",
	},
	{
		name:       "runtimevar",
		goDemoPath: "/demo/runtimevar/demo_runtimevar.go",
		demoURL:    "/demo/runtimevar/",
	},
	{
		name:       "secrets",
		goDemoPath: "/demo/secrets/demo_secrets.go",
		demoURL:    "/demo/secrets/",
	},
}

func registerDemoCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {

	demoCmd := &cobra.Command{
		Use:   "demo",
		Short: "TODO Manage demos",
		Long:  "TODO more about demos",
	}

	demoListCmd := &cobra.Command{
		Use:   "list",
		Short: "TODO List available demos",
		Long:  "TODO more about listing demos",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return listDemos(pctx)
		},
	}
	demoCmd.AddCommand(demoListCmd)

	var force bool
	demoAddCmd := &cobra.Command{
		Use:   "add DEMO",
		Short: "TODO Add a demo",
		Long:  "TODO more about adding a demo",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return addDemo(ctx, pctx, args[0], force)
		},
	}
	demoAddCmd.Flags().BoolVar(&force, "force", false, "re-add even the demo even if it has already been added, overwriting previous files")
	demoCmd.AddCommand(demoAddCmd)
	rootCmd.AddCommand(demoCmd)
}

func listDemos(pctx *processContext) error {
	// Compute a sorted slice of available demos for usage.
	var avail []string
	for _, demo := range allDemos {
		avail = append(avail, demo.name)
	}
	sort.Strings(avail)
	pctx.Println(strings.Join(avail, "\n"))
	return nil
}

func addDemo(ctx context.Context, pctx *processContext, demoToAdd string, force bool) error {
	for _, demo := range allDemos {
		if demo.name == demoToAdd {
			return instantiateDemo(pctx, demo, force)
		}
	}
	return xerrors.Errorf("%q is not a supported demo; try 'demo list' to see available demos")
}

// instantiateDemo does all of the work required to add a demo of a
// portable API to the user's project.
// TODO(rvangent): It currently copies a single source code file. It should
// additionally iterate over existing biomes, adding a config entry and possibly
// Terraform files.
func instantiateDemo(pctx *processContext, demo *demoInfo, force bool) error {
	pctx.Logf("Adding %q...", demo.name)

	// TODO(rvangent): Consider using materializeTemplateDir here. It can't
	// be used right now because it treats the source files as templates;
	// the demo .go files have embedded templates that shouldn't be
	// processed at copy time.
	// It would also need support for "force".

	dstPath := path.Join(pctx.workdir, filepath.Base(demo.goDemoPath))
	if !force {
		if _, err := os.Stat(dstPath); err == nil {
			return xerrors.Errorf("%q has already been added to your project. Use --force if you want to re-add it, overwriting previous files", demo.name)
		}
	}

	srcFile, err := static.Open(demo.goDemoPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}
	pctx.Logf("  added a demo of %s to your project.", filepath.Base(demo.goDemoPath))
	pctx.Logf("Run 'gocdk serve' and visit http://localhost:8080%s to see a demo of %s functionality.", demo.demoURL, demo.name)
	return nil
}
