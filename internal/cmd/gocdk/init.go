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
	"os"
	"os/exec"
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

	// TODO(light): allow an existing empty directory, for some definition of empty
	if _, err := os.Stat(projectDir); err == nil {
		if !allowExistingDir {
			return xerrors.Errorf("gocdk init: %s already exists", projectDir)
		}
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	if err := os.MkdirAll(projectDir, 0777); err != nil {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	tmplValues := struct {
		ProjectName string
		ModulePath  string
	}{
		ProjectName: filepath.Base(projectDir),
		ModulePath:  modpath,
	}
	for fileName, templateSource := range InitTemplates {
		tmpl := template.Must(template.New(fileName).Parse(templateSource))
		buf := new(bytes.Buffer)
		if err := tmpl.Execute(buf, tmplValues); err != nil {
			return xerrors.Errorf("gocdk init: %w", err)
		}

		fullPath := filepath.Join(projectDir, filepath.FromSlash(fileName))
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

var InitTemplates = map[string]string{

	"README.md": `I'm a readme about using the cli`,

	"Dockerfile": `
# gocdk-image: {{.ProjectName}}
`,

	"go.mod": `module {{.ModulePath}}
`,

	"main.go": `package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/", greet)
	if err := http.ListenAndServe(":" + port, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func greet(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}
`,

	"biomes/dev/biome.json": `{
		"serve_enabled" : true,
		"launcher" : "local"
	}
`,

	"biomes/README.md": `I'm a readme about biomes
`,

	"biomes/dev/main.tf": `
`,

	"biomes/dev/outputs.tf": `
`,

	"biomes/dev/variables.tf": `
`,

	"biomes/dev/secrets.auto.tfvars": `
`,

	".dockerignore": `*.tfvars
`,

	".gitignore": `*.tfvars
`,
}
