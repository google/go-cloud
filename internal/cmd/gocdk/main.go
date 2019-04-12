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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"
)

func main() {
	pctx, err := osProcessContext()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)

	}
	debug := false
	err = run(context.Background(), pctx, os.Args[1:], &debug)
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "%+v\n", err)
		} else {
			// TODO(light): format error message parts one per line?
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		if xerrors.As(err, new(usageError)) {
			os.Exit(64)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, pctx *processContext, args []string, debug *bool) error {
	globalFlags := newFlagSet(pctx, "gocdk")
	globalFlags.BoolVar(debug, "debug", false, "show verbose error messages")
	if err := globalFlags.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk: %w", err)
	}
	if globalFlags.NArg() == 0 {
		return usagef("gocdk COMMAND ...")
	}

	cmdArgs := globalFlags.Args()[1:]
	switch cmdName := globalFlags.Arg(0); cmdName {
	case "init":
		return init_(ctx, pctx, cmdArgs)
	case "serve":
		return serve(ctx, pctx, cmdArgs)
	case "add":
		return add(ctx, pctx, cmdArgs)
	case "deploy":
		return deploy(ctx, pctx, cmdArgs)
	case "build":
		return build(ctx, pctx, cmdArgs)
	case "apply":
		return apply(ctx, pctx, cmdArgs)
	case "launch":
		return launch(ctx, pctx, cmdArgs)
	default:
		// TODO(light): We should do spell-checking/fuzzy-matching.
		return usagef("unknown gocdk command %s", cmdName)
	}
}

// processContext is the state that gocdk uses to run. It is collected in
// this struct to avoid obtaining this from globals for simpler testing.
type processContext struct {
	workdir string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

// osProcessContext returns the default process context from global variables.
func osProcessContext() (*processContext, error) {
	workdir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &processContext{
		workdir: workdir,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}, nil
}

// findBiomeDir returns the path to the named biome.
func findBiomeDir(moduleRoot, name string) string {
	return filepath.Join(moduleRoot, "biomes", name)
}

// findModuleRoot searches the given directory and those above it for the Go
// module root.
func findModuleRoot(ctx context.Context, dir string) (string, error) {
	c := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}")
	c.Dir = dir
	output, err := c.Output()
	if err != nil {
		return "", xerrors.Errorf("find module root for %s: %w", dir, err)
	}
	output = bytes.TrimSuffix(output, []byte("\n"))
	if len(output) == 0 {
		return "", xerrors.Errorf("find module root for %s: no module found", dir, err)
	}
	return string(output), nil
}
