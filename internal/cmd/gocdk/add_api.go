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
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

type portableAPIInfo struct {
	name       string // the portable API name
	goDemoPath string // the path to the .go file to add to the project for static.Open
	demoURL    string // the URL for the demo
}

var portableAPIs = []*portableAPIInfo{
	{
		name:       "blob",
		goDemoPath: "/demo/blob/demo_blob.go",
		demoURL:    "/demo/blob/",
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

func addPortableAPI(ctx context.Context, pctx *processContext, args []string) error {
	// Compute a sorted slice of available portable APIs for usage.
	var avail []string
	for _, api := range portableAPIs {
		avail = append(avail, api.name)
	}
	sort.Strings(avail)
	usageMsg := fmt.Sprintf("gocdk add-api <%s>", strings.Join(avail, "|"))

	f := newFlagSet(pctx, "add-api")
	force := f.Bool("force", false, "re-add even the portable API even if it has already been added, overwriting previous files")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("%s: %w", usageMsg, err)
	}

	args = f.Args()
	if len(args) != 1 {
		return usagef("%s: expected 1 argument, got %d", usageMsg, len(args))
	}

	for _, api := range portableAPIs {
		if api.name == args[0] {
			return instantiatePortableAPI(pctx, api, *force)
		}
	}
	return xerrors.Errorf("%q is not a supported portable API. Available APIs include: %s.", args[0], strings.Join(avail, ", "))
}

// instantiatePortableAPI does all of the work required to add a demo of a
// portable API to the user's project.
// TODO(rvangent): It currently copies a single source code file. It should
// additionally iterate over existing biomes, adding a config entry and possibly
// Terraform files.
func instantiatePortableAPI(pctx *processContext, api *portableAPIInfo, force bool) error {
	logger := log.New(pctx.stderr, "gocdk: ", log.Ldate|log.Ltime)
	logger.Printf("Adding %q...", api.name)

	dstPath := path.Join(pctx.workdir, filepath.Base(api.goDemoPath))
	if !force {
		if _, err := os.Stat(dstPath); err == nil {
			return xerrors.Errorf("%q has already been added to your project. Use --force if you want to re-add it, overwriting previous files", api.name)
		}
	}

	srcFile, err := static.Open(api.goDemoPath)
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
	logger.Printf("  added %s to your project.", filepath.Base(api.goDemoPath))
	logger.Printf("Run 'gocdk serve' and visit http://localhost:8080%s to see a demo of %s functionality.", api.demoURL, api.name)
	return nil
}
