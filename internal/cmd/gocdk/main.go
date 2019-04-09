package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"golang.org/x/xerrors"
)

func main() {
	pctx, err := osProcessContext()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)

	}
	err = run(context.Background(), pctx, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v\n", err)
		if xerrors.As(err, new(usageError)) {
			os.Exit(64)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, pctx *processContext, args []string) error {
	if len(args) == 0 {
		return usageError("usage: gocdk COMMAND ...")
	}
	switch args[0] {
	case "init":
		return init_(ctx, pctx, args[1:])
	case "serve":
		return serve(ctx, pctx, args[1:])
	case "add":
		return add(ctx, pctx, args[1:])
	case "deploy":
		return deploy(ctx, pctx, args[1:])
	case "build":
		return build(ctx, pctx, args[1:])
	case "apply":
		return apply(ctx, pctx, args[1:])
	case "launch":
		return launch(ctx, pctx, args[1:])
	default:
		// TODO(light): We should do spell-checking/fuzzy-matching.
		return usagef("unknown command %s", args[0])
	}
}

// usageError indicates an invalid invocation of gocdk.
type usageError string

func usagef(format string, args ...interface{}) error {
	return usageError(fmt.Sprintf(format, args...))
}

func (ue usageError) Error() string {
	return string(ue)
}

// processContext is the state that gocdk uses to run. It is collected in
// this struct to avoid obtaining this from globals for simpler testing.
type processContext struct {
	workdir string
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
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}, nil
}

// findModuleRoot searches the given directory and those above it for the Go
// module root.
func findModuleRoot(ctx context.Context, dir string) (string, error) {
	// TODO(light): Write some tests for this.

	c := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}")
	c.Dir = dir
	output, err := c.Output()
	if err != nil {
		return "", xerrors.Errorf("find module root: %w", err)
	}
	output = bytes.TrimSuffix(output, []byte("\n"))
	return string(output), nil
}
