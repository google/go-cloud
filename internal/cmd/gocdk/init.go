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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	slashpath "path"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/xerrors"
)

func init_(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "init")
	var modpath string
	var allowExistingDir bool
	f.StringVar(&modpath, "module-path", "", "the module import path for your "+
		"project's go.mod file (required if project is outside of GOPATH)")
	f.StringVar(&modpath, "m", "", "alias for --module-path")
	// TODO(#1918): Remove this flag when empty directories are allowed; it is
	// currently used to enabled tests to create an empty tempdir and then run
	// "init" on it.
	f.BoolVar(&allowExistingDir, "allow-existing-dir", false, "true to allow initializing an existing directory (contents may be overwritten!)")

	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk init: %w", err)
	}

	if f.NArg() != 1 {
		return usagef("gocdk init [--module-path=MODULE_IMPORT_PATH] PATH_TO_PROJECT_DIR")
	}

	projectDir := pctx.resolve(f.Arg(0))
	if modpath == "" {
		var err error
		modpath, err = inferModulePath(ctx, pctx, projectDir)
		if err != nil {
			// TODO(clausti): return information about how to mitigate this error
			// e.g. tell them to use --module-path to specify it
			return xerrors.Errorf("gocdk init: %w", err)
		}
	}

	// TODO(#1918): allow an existing empty directory, for some definition of empty.
	if _, err := os.Stat(projectDir); err == nil {
		if !allowExistingDir {
			return xerrors.Errorf("gocdk init: %s already exists", projectDir)
		}
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	tmplValues := struct {
		ProjectName string
		ModulePath  string
	}{
		ProjectName: filepath.Base(projectDir),
		ModulePath:  modpath,
	}
	if err := materializeTemplateDir(projectDir, "init", tmplValues); err != nil {
		return xerrors.Errorf("gocdk init: %w", err)
	}
	fmt.Fprintf(pctx.stdout, "Project created at %s with:\n", projectDir)
	fmt.Fprintln(pctx.stdout, "- Go HTTP server")
	fmt.Fprintln(pctx.stdout, "- Dockerfile")
	fmt.Fprintln(pctx.stdout, "- 'dev' biome for local development settings")
	fmt.Fprintln(pctx.stdout)
	fmt.Fprintf(pctx.stdout, "Run `cd %s`, then run:\n", f.Arg(0))
	fmt.Fprintln(pctx.stdout, "- `gocdk serve` to develop locally")
	fmt.Fprintln(pctx.stdout, "- `gocdk demo` to test new APIs")
	fmt.Fprintln(pctx.stdout, "- `gocdk build` to build a Docker container")
	fmt.Fprintln(pctx.stdout, "- `gocdk biome` to create a biome for launch")
	return nil
}

func materializeTemplateDir(dst string, srcRoot string, data interface{}) error {
	dir, err := static.Open(srcRoot)
	if err != nil {
		return xerrors.Errorf("materialize %s at %s: %w", srcRoot, dst, err)
	}
	infos, err := dir.Readdir(-1)
	dir.Close()
	if err != nil {
		return xerrors.Errorf("materialize %s at %s: %w", srcRoot, dst, err)
	}
	if err := os.MkdirAll(dst, 0777); err != nil {
		return xerrors.Errorf("materialize %s at %s: %w", srcRoot, dst, err)
	}
	for _, info := range infos {
		name := info.Name()
		currDst := filepath.Join(dst, name)
		currSrc := slashpath.Join(srcRoot, name)
		if info.IsDir() {
			if err := materializeTemplateDir(currDst, currSrc, data); err != nil {
				return err
			}
			continue
		}
		f, err := static.Open(currSrc)
		if err != nil {
			return xerrors.Errorf("materialize %s at %s: %w", currSrc, currDst, err)
		}
		templateSource, err := ioutil.ReadAll(f)
		f.Close()
		if err != nil {
			return xerrors.Errorf("materialize %s at %s: %w", currSrc, currDst, err)
		}
		tmpl, err := template.New(name).Parse(string(templateSource))
		if err != nil {
			return xerrors.Errorf("materialize %s at %s: %w", currSrc, currDst, err)
		}
		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, data); err != nil {
			return xerrors.Errorf("materialize %s at %s: %w", currSrc, currDst, err)
		}
		if err := ioutil.WriteFile(currDst, buf.Bytes(), 0666); err != nil {
			return xerrors.Errorf("materialize %s at %s: %w", currSrc, currDst, err)
		}
	}
	return nil
}

// inferModulePath will check the default GOPATH to attempt to infer the module
// import path for the project.
func inferModulePath(ctx context.Context, pctx *processContext, projectDir string) (string, error) {
	// TODO(issue #2016): Add tests for init behavior when module-path is not given.
	cmd := exec.CommandContext(ctx, "go", "env", "GOPATH")
	cmd.Dir = pctx.workdir
	cmd.Env = pctx.env
	gopath, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("infer module path: %w", err)
	}
	// Check if the projectDir is relative to GOPATH.
	rel, err := filepath.Rel(strings.TrimSuffix(string(gopath), "\n"), projectDir)
	if err != nil {
		return "", xerrors.Errorf("infer module path: %w", err)
	}
	inGOPATH := !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if !inGOPATH {
		// If the project dir is outside of GOPATH, we can't infer the module import path.
		return "", xerrors.Errorf("infer module path: %s not in GOPATH", projectDir)
	}
	return filepath.ToSlash(rel), nil
}
