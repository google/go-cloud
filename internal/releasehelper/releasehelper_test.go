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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The following directory/file structure is created. ROOT is the root temp
// directory created by the test.
//
// ROOT/go.mod          <-- main go.mod for gocloud.dev
// ROOT/submod/go.mod   <-- go.mod for a submodule of gocloud.dev
// ROOT/samples/go.mod  <-- go.mod for "samples" that include both of the
//                          other modules
var mainGomod = []byte("module gocloud.dev\n")

var submodGomod = []byte(`module gocloud.dev/submod

require (
	gocloud.dev v0.15.0
)
`)

var samplesGomod = []byte(`module gocloud.dev/samples

require (
	gocloud.dev v0.15.0
	gocloud.dev/submod v0.15.0
)
`)

func createFilesForTest(root string) error {
	if err := ioutil.WriteFile(filepath.Join(root, "go.mod"), mainGomod, 0666); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "submod"), 0766); err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(root, "submod", "go.mod"), submodGomod, 0666); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "samples"), 0766); err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(root, "samples", "go.mod"), samplesGomod, 0666); err != nil {
		return err
	}
	return nil
}

func Test(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "releasehelper_test")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("temp dir:", tempDir)
	if err := createFilesForTest(tempDir); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Add replace lines and expect to find them.
	gomodAddReplace("samples")

	samplesGomod := filepath.Join("samples", "go.mod")
	c, err := ioutil.ReadFile(samplesGomod)
	if err != nil {
		t.Fatal(err)
	}

	replaceLines := []string{
		"replace gocloud.dev => " + filepath.Clean("../"),
		"replace gocloud.dev/submod => " + filepath.Clean("../submod")}

	for _, line := range replaceLines {
		if !strings.Contains(string(c), line) {
			t.Errorf("Expected to find '%s' in samples/go.mod", line)
		}
	}

	// Drop replace lines and expect not to find them.
	gomodDropReplace("samples")
	c, err = ioutil.ReadFile(samplesGomod)
	if err != nil {
		t.Fatal(err)
	}

	for _, line := range replaceLines {
		if strings.Contains(string(c), line) {
			t.Errorf("Expected to not find '%s' in samples/go.mod", line)
		}
	}

	// Set new version and check it was set as expected.
	gomodSetVersion("samples", "v1.8.99")
	c, err = ioutil.ReadFile(samplesGomod)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(c), "gocloud.dev v1.8.99") || !strings.Contains(string(c), "gocloud.dev/submod v1.8.99") {
		t.Error("New versions for require not found in samples/go.mod")
	}
}
