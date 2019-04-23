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
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/xerrors"
)

func serve(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "serve")
	address := f.String("address", "localhost:8080", "address to serve on")
	biome := f.String("biome", "dev", "biome to apply and use configuration from")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk serve: %w", err)
	}
	if f.NArg() != 0 {
		return usagef("gocdk serve [options]")
	}

	// Check first that we're in a Go module.
	moduleRoot, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}

	// Verify that biome configuration permits serving.
	biomeConfig, err := readBiomeConfig(moduleRoot, *biome)
	if xerrors.As(err, new(*biomeNotFoundError)) {
		// TODO(light): Keep err in formatting chain for debugging.
		return xerrors.Errorf("gocdk serve: biome configuration not found for %s. "+
			"Make sure that %s exists and has `\"serve_enabled\": true`.",
			*biome, filepath.Join(findBiomeDir(moduleRoot, *biome), biomeConfigFileName))
	}
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}
	if biomeConfig.ServeEnabled == nil || !*biomeConfig.ServeEnabled {
		return xerrors.Errorf("gocdk serve: biome %s has not enabled serving. "+
			"Add `\"serve_enabled\": true` to %s and try again.",
			*biome, filepath.Join(findBiomeDir(moduleRoot, *biome), biomeConfigFileName))
	}

	// Start main serve loop.
	logger := log.New(pctx.stderr, "gocdk: ", log.Ldate|log.Ltime)
	if err := serveLoop(ctx, pctx, logger, *address, moduleRoot, *biome); err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}
	return nil
}

func serveLoop(ctx context.Context, pctx *processContext, logger *log.Logger, address string, moduleRoot, biome string) error {
	// Log biome that is being used.
	logger.Printf("Preparing to serve %s...", biome)

	// Create a temporary build directory.
	buildDir, err := ioutil.TempDir("", "gocdk-build")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(buildDir); err != nil {
			logger.Printf("Cleaning build: %v", err)
		}
	}()

	// Apply Terraform configuration in biome.
	if err := apply(ctx, pctx, []string{biome}); err != nil {
		return err
	}

	// Build and run the server.
	// TODO(light): Bind host is ignored for now because eventually this
	// subprocess will be reverse proxied. The proxy should respect the host.
	_, portString, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	port, err := net.LookupPort("tcp", portString)
	if err != nil {
		return xerrors.Errorf("resolve port for %q: %w", address, err)
	}
	alloc := &serverAlloc{
		exePath: filepath.Join(buildDir, "server"),
		port:    port,
	}
	logger.Println("Building server...")
	if err := buildForServe(ctx, pctx, moduleRoot, alloc.exePath); err != nil {
		return err
	}
	process, err := alloc.start(ctx, pctx, logger)
	if err != nil {
		return err
	}
	logger.Printf("Serving at %s\nUse Ctrl-C to stop", alloc.url(""))
	// TODO(light): Loop on filesystem change.
	<-ctx.Done()
	logger.Println("Shutting down...")
	if err := signalGracefulShutdown(process.Process); err != nil {
		process.Process.Kill()
	}
	process.Wait()
	return nil
}

// buildForServe runs Wire and `go build` at moduleRoot to create exePath.
func buildForServe(ctx context.Context, pctx *processContext, moduleRoot string, exePath string) error {
	moduleEnv := append(append([]string(nil), pctx.env...), "GO111MODULE=on")

	if wireExe, err := exec.LookPath("wire"); err == nil {
		// TODO(light): Only run Wire if needed, but that requires source analysis.
		wireCmd := exec.CommandContext(ctx, wireExe, "./...")
		wireCmd.Dir = moduleRoot
		wireCmd.Env = moduleEnv
		// TODO(light): Collect build logs into error.
		wireCmd.Stdout = pctx.stderr
		wireCmd.Stderr = pctx.stderr
		if err := wireCmd.Run(); err != nil {
			return xerrors.Errorf("build server: wire: %w", err)
		}
	}

	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", exePath)
	buildCmd.Dir = moduleRoot
	buildCmd.Env = moduleEnv
	// TODO(light): Collect build logs into error.
	buildCmd.Stdout = pctx.stderr
	buildCmd.Stderr = pctx.stderr
	if err := buildCmd.Run(); err != nil {
		return xerrors.Errorf("build server: go build: %w", err)
	}

	return nil
}

// serverAlloc stores the built executable path and port for a single instance
// of the user's application server.
type serverAlloc struct {
	exePath string
	port    int
}

// url returns the URL to connect to the server with.
func (alloc *serverAlloc) url(path string) *url.URL {
	if path == "" {
		path = "/"
	}
	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", alloc.port),
		Path:   path,
	}
}

// start starts the server process specified by the alloc and waits for
// it to become healthy.
func (alloc *serverAlloc) start(ctx context.Context, pctx *processContext, logger *log.Logger) (*exec.Cmd, error) {
	// Run server.
	logger.Print("Starting server...")
	process := exec.Command(alloc.exePath, fmt.Sprintf("--address=localhost:%d", alloc.port))
	process.Stdout = pctx.stdout
	process.Stderr = pctx.stderr
	if err := process.Start(); err != nil {
		return nil, xerrors.Errorf("start server: %w", err)
	}

	// Server must report alive within 30 seconds.
	// TODO(light): Also wait on process to see if it exits early.
	aliveCtx, aliveCancel := context.WithTimeout(ctx, 30*time.Second)
	err := waitForHealthy(aliveCtx, alloc.url("/healthz/liveness"))
	aliveCancel()
	if err != nil {
		process.Process.Kill() // Send SIGKILL; no graceful shutdown.
		process.Wait()
		return nil, xerrors.Errorf("start server: %w", err)
	}

	// Wait for server to be ready.
	logger.Printf("Waiting for server %s to report ready...", alloc.url("/"))
	err = waitForHealthy(ctx, alloc.url("/healthz/readiness"))
	if err != nil {
		if err := signalGracefulShutdown(process.Process); err != nil {
			process.Process.Kill()
		}
		process.Wait()
		return nil, xerrors.Errorf("start server: %w", err)
	}
	return process, nil
}

// waitForHealthy polls a URL repeatedly until the server responds with a
// non-server-error status, the context is canceled, or the context's deadline
// is met. waitForHealthy returns an error in the latter two cases.
func waitForHealthy(ctx context.Context, u *url.URL) error {
	// Create health request.
	// (Avoiding http.NewRequest to not reparse the URL.)
	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
	}
	req = req.WithContext(ctx)

	// Poll for healthy.
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		// Check response. Allow 200-level (success) or 400-level (client error)
		// status codes. The latter is permitted for the case where the application
		// doesn't serve explicit health checks.
		resp, err := http.DefaultClient.Do(req)
		if err == nil && (statusCodeInRange(resp.StatusCode, 200) || statusCodeInRange(resp.StatusCode, 400)) {
			return nil
		}
		// Wait for the next tick.
		select {
		case <-tick.C:
		case <-ctx.Done():
			return xerrors.Errorf("wait for healthy: %w", ctx.Err())
		}
	}
}

// statusCodeInRange reports whether the given HTTP status code is in the range
// [start, start+100).
func statusCodeInRange(statusCode, start int) bool {
	if start < 100 || start%100 != 0 {
		panic("statusCodeInRange start must be a multiple of 100")
	}
	return start <= statusCode && statusCode < start+100
}
