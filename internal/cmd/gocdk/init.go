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
	"bytes"
	"context"
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"gocloud.dev/internal/cmd/gocdk/internal/templates"
	"golang.org/x/xerrors"
)

func init_(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "init")
	var modpath string
	f.StringVar(&modpath, "module-path", "", "the module path for your project's go.mod file")
	f.StringVar(&modpath, "m", "", "alias for --module-path")

	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk init: %w", err)
	}

	if f.NArg() != 1 {
		return usagef("gocdk init [options] PATH")
	}

	path := pctx.resolve(f.Arg(0))
	if modpath == "" {
		var err error
		modpath, err = inferModulePath(ctx, pctx, path)
		if err != nil {
			// TODO(clausti): return information about how to mitigate this error
			// e.g. tell them to use --module-path to specify it
			return xerrors.Errorf("gocdk init: %w", err)
		}
	}

	// TODO(light): allow an existing empty directory, for some definition of empty
	if _, err := os.Stat(path); err == nil {
		return xerrors.Errorf("gocdk init: %s already exists", path)
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	if err := os.MkdirAll(path, 0777); err != nil {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	tmplValues := struct {
		ProjectName string
		ModulePath  string
	}{
		ProjectName: filepath.Base(path),
		ModulePath:  modpath,
	}
	for name, templateSource := range templates.InitTemplates {
		tmpl := template.Must(template.New("").Parse(templateSource))
		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, tmplValues); err != nil {
			return xerrors.Errorf("gocdk init: %w", err)
		}

		fullPath := filepath.Join(path, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0777); err != nil {
			return xerrors.Errorf("gocdk init: %w", err)
		}
		if err := ioutil.WriteFile(fullPath, buf.Bytes(), 0666); err != nil {
			return xerrors.Errorf("gocdk init: %w", err)
		}
	}
	return nil
}

// inferModulePath will check the default GOPATH to attempt to infer the module
// import path for the project.
func inferModulePath(ctx context.Context, pctx *processContext, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "GOPATH")
	cmd.Dir = pctx.workdir
	cmd.Env = pctx.env
	gopath, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("infer module path: %w", err)
	}
	//check if path is relative to gopath
	rel, err := filepath.Rel(strings.TrimSuffix(string(gopath), "\n"), path)
	if err != nil {
		return "", xerrors.Errorf("infer module path: %w", err)
	}
	inGOPATH := !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if !inGOPATH {
		return "", xerrors.Errorf("infer module path: %s not in GOPATH", path)
	}
	return filepath.ToSlash(rel), nil
}
