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
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildForServe(t *testing.T) {
	// TODO(light): This test is not hermetic because it brings
	// in an external module.
	const content = `package main
import "fmt"
func main() { fmt.Println("Hello, World!") }`

	ctx := context.Background()
	pctx, cleanup, err := newTestProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Overwrite main.go with content that just prints something out.
	if err := ioutil.WriteFile(filepath.Join(pctx.workdir, "main.go"), []byte(content), 0666); err != nil {
		t.Fatal(err)
	}
	exePath := filepath.Join(pctx.workdir, "hello")
	if runtime.GOOS == "windows" {
		exePath += ".EXE"
	}

	// Build program.
	if err := buildForServe(ctx, pctx, pctx.workdir, exePath); err != nil {
		t.Fatal("buildForServe(...):", err)
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
