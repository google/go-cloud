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
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gocloud.dev/gcp"
	"golang.org/x/oauth2/google"
	"golang.org/x/xerrors"
)

// generate_static converts the files in static/_assets into constants in
// the static package (in static/vfsdata.go).
//go:generate go run generate_static.go

func main() {
	os.Exit(doit())
}

func doit() int {
	pctx, err := newOSProcessContext()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1

	}
	ctx, done := withInterrupt(context.Background())
	err = run(ctx, pctx, os.Args[1:])
	done()
	if err != nil {
		return 1
	}
	return 0
}

func run(ctx context.Context, pctx *processContext, args []string) error {
	var rootCmd = &cobra.Command{
		Use: "gocdk",
		// Short is not used since this is the root command.
		Long: `gocdk can initialize a new project, add deployment targets (known as "biomes"),
add demos of Go CDK portable types, provision cloud resources and use them in the demos,
build the application into a Docker image, deploy the Docker image to a chosen Cloud provider,
and serve the application locally with live code reloading.`,
		// We want to print usage for any command/flag parsing errors, but
		// suppress usage for random errors returned by a command's RunE.
		// This function gets called right before RunE, so suppress from now on.
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
		},
	}

	registerInitCmd(ctx, pctx, rootCmd)
	registerServeCmd(ctx, pctx, rootCmd)
	registerDemoCmd(ctx, pctx, rootCmd)
	registerBiomeCmd(ctx, pctx, rootCmd)
	registerResourceCmd(ctx, pctx, rootCmd)
	registerDeployCmd(ctx, pctx, rootCmd)
	registerBuildCmd(ctx, pctx, rootCmd)

	rootCmd.SetArgs(args)
	rootCmd.SetOutput(pctx.stderr)
	return rootCmd.Execute()
}

// processContext is the state that gocdk uses to run. It is collected in
// this struct to avoid obtaining this from globals for simpler testing.
type processContext struct {
	workdir string
	env     []string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	outlog  *log.Logger
	errlog  *log.Logger
}

// newOSProcessContext returns the default processContext from global variables.
func newOSProcessContext() (*processContext, error) {
	workdir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return newProcessContext(workdir, os.Stdin, os.Stdout, os.Stderr), nil
}

// newTestProcessContext returns a processContext for use in tests.
func newTestProcessContext(workdir string) *processContext {
	return newProcessContext(workdir, strings.NewReader(""), ioutil.Discard, ioutil.Discard)
}

// newProcessContext returns a processContext.
func newProcessContext(workdir string, stdin io.Reader, stdout, stderr io.Writer) *processContext {
	return &processContext{
		workdir: workdir,
		env:     os.Environ(),
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		outlog:  log.New(stdout, "", 0),
		errlog:  log.New(stderr, "gocdk: ", 0),
	}
}

// overrideEnv returns a copy of env that has vars appended to the end.
// It will not modify env's backing array.
func overrideEnv(env []string, vars ...string) []string {
	// Setting the slice's capacity to length ensures that a new backing array
	// is allocated if len(vars) > 0.
	return append(env[:len(env):len(env)], vars...)
}

// resolve joins path with pctx.workdir if path is relative. Otherwise,
// it returns path.
func (pctx *processContext) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(pctx.workdir, path)
}

// gcpCredentials returns the credentials to use for this process context.
func (pctx *processContext) gcpCredentials(ctx context.Context) (*google.Credentials, error) {
	// TODO(light): google.DefaultCredentials uses Getenv directly, so it is
	// difficult to disentangle to use processContext.
	return gcp.DefaultCredentials(ctx)
}

// Logf writes to the pctx's stderr.
func (pctx *processContext) Logf(format string, a ...interface{}) {
	pctx.errlog.Printf(format, a...)
}

// Printf writes to the pctx's stdout.
func (pctx *processContext) Printf(format string, a ...interface{}) {
	pctx.outlog.Printf(format, a...)
}

// Println writes to the pctx's stdout.
func (pctx *processContext) Println(a ...interface{}) {
	pctx.outlog.Println(a...)
}

// ModuleRoot searches the given directory and those above it for the project
// root, based on the existence of a "go.mod" file and a "biomes" directory.
func (pctx *processContext) ModuleRoot(ctx context.Context) (string, error) {
	for dir, prevDir := pctx.workdir, ""; ; dir = filepath.Dir(dir) {
		if dir == prevDir {
			return "", xerrors.Errorf("couldn't find a Go CDK project root at or above %s", pctx.workdir)
		}
		prevDir = dir
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			continue
		}
		if _, err := os.Stat(biomesRootDir(dir)); err != nil {
			continue
		}
		return dir, nil
	}
}

// NewCommand creates a new exec.Cmd based on this processContext.
//
// dir may be empty; it defaults to pctx.workdir.
//
// Note that this function sets cmd.Stdout, and that Cmd.Output requires
// Stdout to be nil; you may need to set it back to nil explicitly (or
// don't use this function). Similarly for Cmd.CombinedOutput, for both
// Stdout and Stderr.
func (pctx *processContext) NewCommand(ctx context.Context, dir, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	if dir == "" {
		cmd.Dir = pctx.workdir
	} else {
		cmd.Dir = dir
	}
	cmd.Env = pctx.env
	cmd.Stdin = pctx.stdin
	cmd.Stdout = pctx.stdout
	cmd.Stderr = pctx.stderr
	return cmd
}

// withInterrupt returns a copy of parent with a new Done channel. The returned
// context's Done channel will be closed when the process receives an interrupt
// signal, the parent context's Done channel is closed, or the stop function is
// called, whichever comes first.
//
// The stop function releases resources and stops listening for signals, so code
// should call it as soon as the operation using the context completes.
func withInterrupt(parent context.Context) (_ context.Context, stop func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, interruptSignals()...)
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		select {
		case <-sig:
			cancel()
		case <-done:
		}
	}()
	return ctx, func() {
		cancel()
		signal.Stop(sig)
		close(done)
	}
}
