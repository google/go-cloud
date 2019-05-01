// Copyright 2019 The Go Cloud Authors
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

type portableTypeInfo struct {
	name       string // the portable type name
	goDemoPath string // the path to the .go file to add to the project for static.Open
	demoURL    string // the URL for the demo
}

var portableTypes = []*portableTypeInfo{
	{
		name:       "blob.Bucket",
		goDemoPath: "/demo/blob.bucket/demo_blob_bucket.go",
		demoURL:    "/demo/blob.bucket/",
	},
}

// TODO(rvangent): Add tests for add(), including for each supported portableType.

func add(ctx context.Context, pctx *processContext, args []string) error {
	// Compute a sorted slice of available portable types for usage.
	var avail []string
	for _, pt := range portableTypes {
		avail = append(avail, pt.name)
	}
	sort.Strings(avail)
	usageMsg := fmt.Sprintf("gocdk add <%s>", strings.Join(avail, "|"))

	f := newFlagSet(pctx, "add")
	force := f.Bool("force", false, "re-add even the portable type even if it has already been added, overwriting previous files")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("%s: %w", usageMsg, err)
	}

	args = f.Args()
	if len(args) != 1 {
		return usagef("%s: expected 1 argument, got %d", usageMsg, len(args))
	}

	for _, pt := range portableTypes {
		if pt.name == args[0] {
			return instantiatePortableType(pctx, pt, *force)
		}
	}
	return xerrors.Errorf("%q is not a supported portable type. Available types include: %s.", args[0], strings.Join(avail, ", "))
}

// instantiatePortableType does all of the work required to add a portable type
// to the user's project.
// TODO(rvangent): It currently copies a single source code file. It should
// additionally iterate over existing biomes, adding a config entry and possibly
// Terraform files.
func instantiatePortableType(pctx *processContext, pt *portableTypeInfo, force bool) error {
	logger := log.New(pctx.stderr, "gocdk: ", log.Ldate|log.Ltime)
	logger.Printf("Adding %q...", pt.name)

	dstPath := path.Join(pctx.workdir, filepath.Base(pt.goDemoPath))
	if !force {
		if _, err := os.Stat(dstPath); err == nil {
			return xerrors.Errorf("%q has already been added to your project. Use --force if you want to re-add it, overwriting previous files", pt.name)
		}
	}

	srcFile, err := static.Open(pt.goDemoPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// TODO(rvangent): This creates the demo file at the top level of the user's
	// project. Should we create a subdirectory/sub-package?
	destFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}
	logger.Printf("  added %s to your project.", filepath.Base(pt.goDemoPath))
	logger.Printf("Run 'gocdk serve' and visit http://127.0.0.1:8080%s to see a demo of %s functionality.", pt.demoURL, pt.name)
	return nil
}
