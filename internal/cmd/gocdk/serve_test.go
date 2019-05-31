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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"gocloud.dev/health"
	"golang.org/x/xerrors"
)

func TestBuildForServe(t *testing.T) {
	t.Run("NoWire", func(t *testing.T) {
		testBuildForServe(t, map[string]string{
			"main.go": `package main
import "fmt"
func main() { fmt.Println("Hello, World!") }`,
		})
	})
	t.Run("Wire", func(t *testing.T) {
		if _, err := exec.LookPath("wire"); err != nil {
			// Wire tool is run unconditionally. Needed for both tests.
			t.Skip("wire not found:", err)
		}
		// TODO(light): This test is not hermetic because it brings
		// in an external module.
		testBuildForServe(t, map[string]string{
			"main.go": `package main
import "fmt"
func main() { fmt.Println(greeting()) }`,
			"setup.go": `// +build wireinject

package main
import "github.com/google/wire"
func greeting() string {
	wire.Build(wire.Value("Hello, World!"))
	return ""
}`,
		})
	})
}

func testBuildForServe(t *testing.T, files map[string]string) {
	dir, cleanup, err := newTestModule()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for name, content := range files {
		if err := ioutil.WriteFile(filepath.Join(dir, name), []byte(content), 0666); err != nil {
			t.Fatal(err)
		}
	}
	pctx := newTestProcessContext(dir)
	exePath := filepath.Join(dir, "hello")
	if runtime.GOOS == "windows" {
		exePath += ".EXE"
	}

	// Build program.
	if err := buildForServe(context.Background(), pctx, dir, exePath); err != nil {
		t.Error("buildForServe(...):", err)
	}

	// Run program and check output to ensure correctness.
	got, err := exec.Command(exePath).Output()
	if err != nil {
		t.Fatal(err)
	}
	const want = "Hello, World!\n"
	if string(got) != want {
		t.Errorf("program output = %q; want %q", got, want)
	}
}

func TestWaitForHealthy(t *testing.T) {
	t.Run("ImmediateHealthy", func(t *testing.T) {
		srv := httptest.NewServer(new(health.Handler))
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if err := waitForHealthy(context.Background(), u); err != nil {
			t.Error("waitForHealthy returned error:", err)
		}
	})
	t.Run("404", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if err := waitForHealthy(context.Background(), u); err != nil {
			t.Error("waitForHealthy returned error:", err)
		}
	})
	t.Run("SecondTimeHealthy", func(t *testing.T) {
		handler := new(health.Handler)
		handler.Add(new(secondTimeHealthy))
		srv := httptest.NewServer(handler)
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}

		if err := waitForHealthy(context.Background(), u); err != nil {
			t.Error("waitForHealthy returned error:", err)
		}
	})
	t.Run("ContextCancelled", func(t *testing.T) {
		handler := new(health.Handler)
		handler.Add(constHealthChecker{xerrors.New("bad")})
		srv := httptest.NewServer(handler)
		defer srv.Close()
		u, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)

		err = waitForHealthy(ctx, u)
		cancel()
		if err == nil {
			t.Error("waitForHealthy did not return error")
		}
	})
}

type constHealthChecker struct {
	err error
}

func (c constHealthChecker) CheckHealth() error {
	return c.err
}

type secondTimeHealthy struct {
	mu            sync.Mutex
	firstReported bool
}

func (c *secondTimeHealthy) CheckHealth() error {
	defer c.mu.Unlock()
	c.mu.Lock()
	if !c.firstReported {
		c.firstReported = true
		return xerrors.New("check again later")
	}
	return nil
}
