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
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gocloud.dev/internal/cmd/gocdk/internal/probe"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func registerServeCmd(ctx context.Context, pctx *processContext, rootCmd *cobra.Command) {
	var opts serveOptions
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "TODO Run an auto-reloading local server",
		Long:  "TODO more about serve",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return serve(ctx, pctx, &opts)
		},
	}
	serveCmd.Flags().StringVar(&opts.address, "address", "localhost:8080", "`host:port` address to serve on")
	serveCmd.Flags().StringVar(&opts.biome, "biome", "dev", "`name` of biome to apply and use configuration from")
	serveCmd.Flags().DurationVar(&opts.pollInterval, "poll-interval", 500*time.Millisecond, "time between checking project directory for changes")
	rootCmd.AddCommand(serveCmd)
}

func serve(ctx context.Context, pctx *processContext, opts *serveOptions) error {
	// Check first that we're in a Go module.
	var err error
	opts.moduleRoot, err = pctx.ModuleRoot(ctx)
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}

	// Verify that biome configuration permits serving.
	biomePath, err := biomeDir(opts.moduleRoot, opts.biome)
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}
	biomeConfig, err := readBiomeConfig(opts.moduleRoot, opts.biome)
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}
	if biomeConfig.ServeEnabled == nil || !*biomeConfig.ServeEnabled {
		return xerrors.Errorf("gocdk serve: biome %s has not enabled serving. "+
			"Add `\"serve_enabled\": true` to %s and try again.",
			opts.biome, filepath.Join(biomePath, biomeConfigFileName))
	}

	// Start reverse proxy on address.
	proxyListener, err := net.Listen("tcp", opts.address)
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}
	opts.actualAddress = proxyListener.Addr().(*net.TCPAddr)
	myProxy := new(serveProxy)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return runHTTPServer(groupCtx, proxyListener, myProxy)
	})

	// Start main build loop.
	group.Go(func() error {
		return serveBuildLoop(groupCtx, pctx, pctx.errlog, myProxy, opts)
	})
	if err := group.Wait(); err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}
	return nil
}

type serveOptions struct {
	moduleRoot   string
	biome        string
	address      string
	pollInterval time.Duration

	// actualAddress is the local address that the reverse proxy is
	// listening on.
	actualAddress *net.TCPAddr
}

// serveBuildLoop builds and runs the user's server and sets the proxy's
// backend whenever a new built server becomes healthy. This loop will continue
// until ctx's Done channel is closed. serveBuildLoop returns an error only
// if it was unable to start the main build loop.
func serveBuildLoop(ctx context.Context, pctx *processContext, logger *log.Logger, myProxy *serveProxy, opts *serveOptions) error {
	// Listen for SIGUSR1 to trigger rebuild.
	reload, reloadDone := notifyUserSignal1()
	defer reloadDone()

	biomePath, err := biomeDir(opts.moduleRoot, opts.biome)
	if err != nil {
		return xerrors.Errorf("gocdk serve: %w", err)
	}

	// Log biome that is being used.
	logger.Printf("Preparing to serve %s...", opts.biome)

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
	if err := biomeApply(ctx, pctx, opts.biome, false); err != nil {
		return err
	}

	// Build and run the server.
	allocA := &serverAlloc{
		exePath: filepath.Join(buildDir, "serverA"),
		port:    opts.actualAddress.Port + 1,
	}
	allocB := &serverAlloc{
		exePath: filepath.Join(buildDir, "serverB"),
		port:    opts.actualAddress.Port + 2,
	}
	if runtime.GOOS == "windows" {
		allocA.exePath += ".EXE"
		allocB.exePath += ".EXE"
	}
	spareAlloc, liveAlloc := allocA, allocB
	var process *exec.Cmd
loop:
	for first := true; ; first = false {
		// After the first iteration of the loop, each iteration should wait for a
		// change in the filesystem before proceeding.
		if !first {
			watchCtx, cancelWatch := context.WithCancel(ctx)
			watchDone := make(chan error)
			go func() {
				watchDone <- waitForDirChange(watchCtx, opts.moduleRoot, opts.pollInterval)
			}()
			select {
			case <-reload:
				// Manual rebuild signal.
				cancelWatch()
				<-watchDone // Wait on watch goroutine.
			case err := <-watchDone:
				cancelWatch()
				if err != nil {
					if !xerrors.Is(err, context.DeadlineExceeded) && !xerrors.Is(err, context.Canceled) {
						logger.Printf("Watch failed: %v", err)
					}
					break loop
				}
			}

			logger.Println("Refreshing biome...")
			if err := refreshBiome(ctx, opts.moduleRoot, opts.biome, pctx.env); err != nil {
				// The Terraform configuration succeeded at least once, so Terraform
				// outputs will still be somewhat consistent. We don't want to stop the
				// build in case communicating with the cloud is flaky.
				logger.Printf("WARNING: %v", err)
			}
		}

		// Read output from Terraform built during apply or refresh.
		tfOutput, err := tfReadOutput(ctx, biomePath, pctx.env)
		if err != nil {
			logger.Printf("Terraform: %v", err)
			if process == nil {
				myProxy.setBuildError(err)
			}
			continue
		}
		procEnv, err := launchEnv(tfOutput)
		if err != nil {
			logger.Printf("Terraform: %v", err)
			if process == nil {
				myProxy.setBuildError(err)
			}
			continue
		}

		// Build and run the server.
		logger.Println("Building server...")
		if err := buildForServe(ctx, pctx, opts.moduleRoot, spareAlloc.exePath); err != nil {
			logger.Printf("Build: %v", err)
			if process == nil {
				myProxy.setBuildError(err)
			}
			continue
		}
		newProcess, err := spareAlloc.start(ctx, pctx, logger, opts.moduleRoot, procEnv)
		if err != nil {
			logger.Printf("Starting server: %v", err)
			if process == nil {
				myProxy.setBuildError(err)
			}
			continue
		}

		// Server started successfully. Cut traffic over.
		myProxy.setBackend(spareAlloc.url(""))
		if process == nil {
			// First time server came up healthy: log greeting message to user.
			proxyURL := "http://" + formatTCPAddr(opts.actualAddress) + "/"
			logger.Printf("Serving at %s\nUse Ctrl-C to stop", proxyURL)
		} else {
			// Iterative build complete, kill old server.
			logger.Print("Reload complete.")
			endServerProcess(process)
		}
		process = newProcess
		spareAlloc, liveAlloc = liveAlloc, spareAlloc
	}
	logger.Println("Shutting down...")
	if process != nil {
		endServerProcess(process)
	}
	return nil
}

// buildForServe runs Wire and `go build` at moduleRoot to create exePath.
// Note that on Windows, exePath must end with .EXE.
func buildForServe(ctx context.Context, pctx *processContext, moduleRoot string, exePath string) error {

	if wireExe, err := exec.LookPath("wire"); err == nil {
		// TODO(light): Only run Wire if needed, but that requires source analysis.
		wireCmd := pctx.NewCommand(ctx, moduleRoot, wireExe, "./...")
		wireCmd.Env = append(wireCmd.Env, "GO111MODULE=on")
		// TODO(light): Collect build logs into error.
		if err := wireCmd.Run(); err != nil {
			return xerrors.Errorf("build server: wire: %w", err)
		}
	}

	buildCmd := pctx.NewCommand(ctx, moduleRoot, "go", "build", "-o", exePath)
	buildCmd.Env = append(buildCmd.Env, "GO111MODULE=on")
	// TODO(light): Collect build logs into error.
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
func (alloc *serverAlloc) start(ctx context.Context, pctx *processContext, logger *log.Logger, workdir string, envOverrides []string) (*exec.Cmd, error) {
	// Run server.
	logger.Print("Starting server...")
	process := pctx.NewCommand(ctx, workdir, alloc.exePath)
	process.Env = append(process.Env, envOverrides...)
	process.Env = append(process.Env, "PORT="+strconv.Itoa(alloc.port))
	if err := process.Start(); err != nil {
		return nil, xerrors.Errorf("start server: %w", err)
	}

	// Server must report alive within 30 seconds.
	// TODO(light): Also wait on process to see if it exits early.
	aliveCtx, aliveCancel := context.WithTimeout(ctx, 30*time.Second)
	err := probe.WaitForHealthy(aliveCtx, alloc.url("/healthz/liveness"))
	aliveCancel()
	if err != nil {
		process.Process.Kill() // Send SIGKILL; no graceful shutdown.
		process.Wait()
		return nil, xerrors.Errorf("start server: %w", err)
	}

	// Wait for server to be ready.
	logger.Printf("Waiting for server %s to report ready...", alloc.url("/"))
	err = probe.WaitForHealthy(ctx, alloc.url("/healthz/readiness"))
	if err != nil {
		endServerProcess(process)
		return nil, xerrors.Errorf("start server: %w", err)
	}
	return process, nil
}

// runHTTPServer runs an HTTP server until ctx's Done channel is closed.
// It returns an error only if the server returned an error before the Done
// channel was closed.
func runHTTPServer(ctx context.Context, l net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler: handler,
	}
	serverDone := make(chan error)
	go func() {
		serverDone <- server.Serve(l)
	}()
	select {
	case err := <-serverDone:
		return err
	case <-ctx.Done():
		// Don't pass ctx, since this context determines the shutdown timeout.
		server.Shutdown(context.Background())
		<-serverDone
		return nil
	}
}

// serveProxy is a reverse HTTP proxy that can hot-swap to other sources.
// The zero value will serve Bad Gateway responses until setBackend or
// setBuildError is called.
type serveProxy struct {
	mu      sync.RWMutex
	backend interface{}
}

// setBackend serves any future requests by reverse proxying to the given URL.
func (p *serveProxy) setBackend(target *url.URL) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backend = httputil.NewSingleHostReverseProxy(target)
}

// setBuildError serves any future requests by serving an Internal Server Error
// with the error's message as the body.
func (p *serveProxy) setBuildError(e error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backend = e
}

func (p *serveProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	b := p.backend
	p.mu.RUnlock()
	switch b := b.(type) {
	case nil:
		http.Error(w, "waiting for initial build...", http.StatusBadGateway)
	case error:
		http.Error(w, b.Error(), http.StatusInternalServerError)
	case http.Handler:
		b.ServeHTTP(w, r)
	default:
		panic("unreachable")
	}
}

// endServerProcess kills and waits for a subprocess to exit.
func endServerProcess(process *exec.Cmd) {
	if err := signalGracefulShutdown(process.Process); err != nil {
		process.Process.Kill()
	}
	process.Wait()
}

// formatTCPAddr converts addr into a "host:port" string usable for a
// URL.
func formatTCPAddr(addr *net.TCPAddr) string {
	if addr.IP.IsUnspecified() {
		return fmt.Sprintf("localhost:%d", addr.Port)
	}
	return addr.String()
}
