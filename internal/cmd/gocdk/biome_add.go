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
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path"
	slashpath "path"
	"path/filepath"
	"text/template"

	"golang.org/x/xerrors"
)

func biomeAdd(ctx context.Context, pctx *processContext, args []string) error {
	// TODO(clausti) interpolate launcher from one supplied as a flag
	f := newFlagSet(pctx, "demo")
	usageMsg := "gocdk biome add BIOME_NAME"
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("%s: %w", usageMsg, err)
	}
	if f.Args()[0] != "add" {
		return usagef("%s: expected add, got %v", usageMsg, f.Args()[0])
	}
	newName := f.Args()[1]
	dstPath := path.Join(pctx.workdir, "biomes", newName)

	tmplDir, err := static.Open("biome_add")
	if err != nil {
		return xerrors.Errorf("biome add: %w", err)
	}
	infos, err := tmplDir.Readdir(-1)
	tmplDir.Close()
	if err != nil {
		return xerrors.Errorf("biome add %v: %w", newName, err)
	}
	if err := os.MkdirAll(dstPath, 0777); err != nil {
		return xerrors.Errorf("biome add %v: %w", newName, err)
	}

	projectDir, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		xerrors.Errorf("biome add: %w", err)
	}
	data := struct {
		ProjectName string
	}{
		ProjectName: filepath.Base(projectDir),
	}

	logger := log.New(pctx.stderr, "gocdk: ", log.Ldate|log.Ltime)
	logger.Printf("Adding new biome '%v'...", newName)

	for _, info := range infos {
		name := info.Name()
		currDst := filepath.Join(dstPath, name)
		currSrc := slashpath.Join("biome_add", name)

		f, err := static.Open(currSrc)
		if err != nil {
			return xerrors.Errorf("biome add %s at %s: %w", currSrc, currDst, err)
		}
		templateSource, err := ioutil.ReadAll(f)
		f.Close()
		if err != nil {
			return xerrors.Errorf("biome add %s at %s: %w", currSrc, currDst, err)
		}
		tmpl, err := template.New(name).Parse(string(templateSource))
		if err != nil {
			return xerrors.Errorf("biome add %s at %s: %w", currSrc, currDst, err)
		}
		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, data); err != nil {
			return xerrors.Errorf("biome add %s at %s: %w", currSrc, currDst, err)
		}
		if err := ioutil.WriteFile(currDst, buf.Bytes(), 0666); err != nil {
			return xerrors.Errorf("biome add %s at %s: %w", currSrc, currDst, err)
		}
	}
	logger.Printf("Successfully added new biome '%v'!", newName)
	return nil
}
