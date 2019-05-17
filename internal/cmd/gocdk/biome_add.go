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
	"path/filepath"

	"golang.org/x/xerrors"
)

func biomeAdd(ctx context.Context, pctx *processContext, args []string) error {
	// TODO(clausti) interpolate launcher from one supplied as a flag
	f := newFlagSet(pctx, "biome")
	const usageMsg = "gocdk biome add"
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("%s: %w", usageMsg, err)
	}
	if f.Arg(0) != "add" {
		return usagef("%s: expected add, got %v", usageMsg, f.Args()[0])
	}
	if f.NArg() != 2 {
		return usagef("%s BIOME_NAME", usageMsg)
	}
	newName := f.Arg(1)

	projectDir, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		xerrors.Errorf("biome add: %w", err)
	}
	dstPath := findBiomeDir(projectDir, newName)
	data := struct {
		ProjectName string
	}{
		ProjectName: filepath.Base(projectDir),
	}

	if err := materializeTemplateDir(dstPath, "biome_add", data); err != nil {
		return xerrors.Errorf("gocdk biome add: %w", err)
	}
	fmt.Println("Successfully added new biome '%v'!", newName)
	return nil
}
