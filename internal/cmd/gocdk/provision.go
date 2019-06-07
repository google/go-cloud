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
	"bytes"
	"context"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/static"
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

// The "provision list" command.
func provisionList(pctx *processContext) error {
	// TODO(rvangent): There must be an easier way to get a listing... add
	// a helper to static.
	dir, err := static.Open(provisionStaticRootDir)
	if err != nil {
		return xerrors.Errorf("provision list: couldn't open static resources: %w", err)
	}
	ptInfos, err := dir.Readdir(-1)
	if err != nil {
		return xerrors.Errorf("provision list: couldn't read static resources: %w", err)
	}
	// The entries at this level are portable APIs, e.g., "blob".
	for _, ptInfo := range ptInfos {
		ptName := ptInfo.Name()
		if !ptInfo.IsDir() {
			continue
		}
		ptTypeDir, err := static.Open(path.Join(provisionStaticRootDir, ptName))
		if err != nil {
			return xerrors.Errorf("provision list: couldn't open static resources: %w", err)
		}
		providerInfos, err := ptTypeDir.Readdir(-1)
		if err != nil {
			return xerrors.Errorf("provision list: couldn't read static resources: %w", err)
		}
		for _, info := range providerInfos {
			if !info.IsDir() {
				continue
			}
			pctx.Println(path.Join(ptName, info.Name()))
		}
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
// TODO(rvangent): Currently all of the variables.tf files have default values, so
//                 "terraform apply" works fine. Add the ability to prompt the user
//                 for things (e.g., GCP project ID), and then instantiate auto.tfvars
//                 files with the values.
func provisionAdd(ctx context.Context, pctx *processContext, biome, typ string) error {
	pctx.Logf("Adding %q to %q...", typ, biome)

	typeParts := strings.Split(typ, "/")
	if len(typeParts) != 2 {
		return xerrors.Errorf("provision add: %q is not a supported type; use 'gocdk provision list' to see available types", typ)
	}

	moduleDir, err := pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("provision add: %w", err)
	}
	dstPath := biomeDir(moduleDir, biome)

	// TODO(rvangent): Use an explicit allowlist instead of just blindly reading "typ".
	srcRoot := path.Join(provisionStaticRootDir, typ)
	srcDir, err := static.Open(srcRoot)
	if err != nil {
		return xerrors.Errorf("provision add: %q is not a supported type; use 'gocdk provision list' to see available types", typ)
	}
	defer srcDir.Close()
	srcInfos, err := srcDir.Readdir(-1)
	if err != nil {
		return xerrors.Errorf("provision add: couldn't read static resources: %w", err)
	}
	for _, srcInfo := range srcInfos {
		name := srcInfo.Name()
		if srcInfo.IsDir() {
			return xerrors.Errorf("provision add: unexpected directory in static resources: %s", name)
		}

		srcFile, err := static.Open(path.Join(srcRoot, name))
		if err != nil {
			return xerrors.Errorf("provision add: couldn't open static resource %q: %w", name, err)
		}
		defer srcFile.Close()
		srcBytes, err := ioutil.ReadAll(srcFile)
		if err != nil {
			return xerrors.Errorf("provision add: couldn't read static resource %q: %w", name, err)
		}
		// TODO(rvangent): There's a bunch of somewhat arcane logic here
		// to figure out what to do with each file. There should be a config
		// file for each directory that tells us what to do with each one.
		// For example:
		// -- By default files are just copied to the destination.
		// -- A config setting that means "treat this as a template".
		// -- A config setting means "insert into this file after this marker".
		// -- A config setting means "append to this file".
		// -- Options for insert/append to tell whether it should be skipped
		//    (i.e., it's been added already), or whether to error.
		// "demo add" and materializeTemplateDir could be updated to use this as well.
		if strings.HasPrefix(name, "main_") {
			if err := insertIntoFile(filepath.Join(dstPath, "main.tf"), "", srcBytes); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
		} else if strings.HasPrefix(name, "variables_") {
			if err := insertIntoFile(filepath.Join(dstPath, "variables.tf"), "", srcBytes); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
		} else if strings.HasPrefix(name, "mainlocal_") {
			const marker = "# DO NOT REMOVE: GO CDK LOCAL WILL BE INSERTED BELOW HERE"
			tmpl, err := template.New(name).Parse(string(srcBytes))
			if err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
			buf := new(bytes.Buffer)
			if err := tmpl.Execute(buf, nil); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
			if err := insertIntoFile(filepath.Join(dstPath, "outputs.tf"), marker, buf.Bytes()); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
		} else if strings.HasPrefix(name, "outputs_") {
			const marker = "# DO NOT REMOVE: DEMO URLs WILL BE INSERTED BELOW HERE"
			tmpl, err := template.New(name).Parse(string(srcBytes))
			if err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
			buf := new(bytes.Buffer)
			if err := tmpl.Execute(buf, nil); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
			if err := insertIntoFile(filepath.Join(dstPath, "outputs.tf"), marker, buf.Bytes()); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
		} else {
			// By default, just copy the file.
			if err := ioutil.WriteFile(filepath.Join(dstPath, name), srcBytes, 0666); err != nil {
				return xerrors.Errorf("provision add: %w", err)
			}
		}
	}

	pctx.Logf("Success!")
	return nil
}

// insertIntoFile inserts toInsert into the file at path.
// If marker is the empty string, it appends to the file.
// If marker is not empty, it inserts at the next line after the marker.
// If toInsert is already in the file at path, insertIntoFile does nothing.
func insertIntoFile(path, marker string, toInsert []byte) error {
	existingContent, err := ioutil.ReadFile(path)
	if err != nil {
		return xerrors.Errorf("couldn't read %s: %w", path, err)
	}

	if idx := bytes.Index(existingContent, toInsert); idx != -1 {
		return nil
	}
	var insertIdx int
	if marker == "" {
		// Append.
		insertIdx = len(existingContent)
	} else {
		markerBytes := []byte(marker)
		insertIdx = bytes.Index(existingContent, markerBytes)
		if insertIdx == -1 {
			return xerrors.Errorf("couldn't find marker %q in %s", marker, path)
		}
		insertIdx += len(markerBytes)
		for existingContent[insertIdx] == '\n' || existingContent[insertIdx] == '\r' {
			insertIdx++
		}
	}
	newContent := append(existingContent[:insertIdx], append(toInsert, existingContent[insertIdx:]...)...)

	// TODO(rvangent): Copy os.FileMode from the current file?
	if err := ioutil.WriteFile(path, newContent, 0666); err != nil {
		return xerrors.Errorf("couldn't update %s: %w", path, err)
	}
	return nil
}
