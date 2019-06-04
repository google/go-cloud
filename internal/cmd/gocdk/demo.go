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
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

type demoInfo struct {
	name       string // the portable API name
	goDemoPath string // the path to the .go file to add to the project for static.Open
	demoURL    string // the URL for the demo
}

// In sorted order.
var allDemos = []string{"blob", "pubsub", "runtimevar", "secrets"}

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
			return listDemos()
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

func listDemos() error {
	fmt.Println(strings.Join(allDemos, "\n"))
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
	return xerrors.Errorf("%q is not a supported demo; try 'gocdk demo list' to see available demos")
}

// instantiateDemo does all of the work required to add a demo of a
// portable API to the user's project.
// TODO(rvangent): It currently copies a single source code file. It should
// additionally iterate over existing biomes, adding a config entry and possibly
// Terraform files.
func instantiateDemo(pctx *processContext, moduleRoot, demo string, force bool) error {
	logger := log.New(pctx.stderr, "gocdk: ", log.Ldate|log.Ltime)
	logger.Printf("Adding %q...", demo)

	// TODO(rvangent): Consider using materializeTemplateDir here. It can't
	// be used right now because it treats the source files as templates;
	// the demo .go files have embedded templates that shouldn't be
	// processed at copy time.
	// It would also need support for "force".

	fileName := fmt.Sprintf("demo_%s.go", demo)
	dstPath := path.Join(moduleRoot, fileName)
	if !force {
		if _, err := os.Stat(dstPath); err == nil {
			return xerrors.Errorf("%q has already been added to your project. Use --force if you want to re-add it, overwriting previous files", demo)
		}
	}
	srcPath := fmt.Sprintf("/demo/%s/%s", demo, fileName)
	srcFile, err := static.Open(srcPath)
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
	logger.Printf("  added %q to your project.", fileName)
	logger.Printf("Run 'gocdk serve' and visit http://localhost:8080/demo/%s to see the demo.", demo)
	return nil
}
