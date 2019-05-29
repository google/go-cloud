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
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	// TODO(light): Test cases:
	// Wrong or missing arguments
	// Empty directory exists
	// Non-empty directory exists

	t.Run("CorrectPreconditions", func(t *testing.T) {
		dir, err := ioutil.TempDir("", testTempDirPrefix)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Error(err)
			}
		}()
		ctx := context.Background()
		pctx := &processContext{
			workdir: dir,
			stdin:   strings.NewReader(""),
			stdout:  ioutil.Discard,
			stderr:  ioutil.Discard,
		}

		const projectName = "myspecialproject"
		if err := run(ctx, pctx, []string{"init", "--module-path=example.com/foo/" + projectName, projectName}); err != nil {
			t.Errorf("run returned error: %+v", err)
		}

		// Check that project directory exists.
		projectDir := filepath.Join(dir, projectName)
		if info, err := os.Stat(projectDir); err != nil {
			t.Fatalf("stat project directory: %+v", err)
		} else if !info.IsDir() {
			t.Fatalf("%s is %v; want directory", projectDir, info.Mode())
		}

		// Dockerfile contains "# gocdk-image: myspecialproject" magic comment.
		dockerfilePath := filepath.Join(projectDir, "Dockerfile")
		dockerfileData, err := ioutil.ReadFile(dockerfilePath)
		if err != nil {
			t.Errorf("read Dockerfile: %+v", err)
		} else {
			const dockerfileComment = "# gocdk-image: " + projectName
			if !containsLine(dockerfileData, dockerfileComment) {
				t.Errorf("%s does not contain magic comment %q. Found:\n%s", dockerfilePath, dockerfileComment, dockerfileData)
			}
		}

		// Contains a valid go.mod file.
		goModList := exec.Command("go", "list", "-m", "-f", "{{with .Error}}{{.Err}}{{else}}OK{{end}}")
		goModList.Dir = projectDir
		if output, err := goModList.CombinedOutput(); err != nil {
			t.Errorf("verifying module: go list returned error: %v. Output:\n%s", err, output)
		} else if !bytes.Equal(output, []byte("OK\n")) {
			t.Errorf("verifying module: unexpected output from go list (want \"OK\"):\n%s", output)
		}

		// Ensure that at least one file exists in dev biome with extension .tf.
		devBiomePath := filepath.Join(projectDir, "biomes", "dev")
		devBiomeContents, err := ioutil.ReadDir(devBiomePath)
		if err != nil {
			t.Error(err)
		} else {
			foundTF := false
			var foundNames []string
			for _, info := range devBiomeContents {
				foundNames = append(foundNames, info.Name())
				if filepath.Ext(info.Name()) == ".tf" {
					foundTF = true
				}
			}
			if !foundTF {
				t.Errorf("%s contains %v; want to contain at least one \".tf\" file", devBiomePath, foundNames)
			}
		}

		// dev biome contains settings we expect.
		devBiomeJSONPath := filepath.Join(devBiomePath, "biome.json")
		devBiomeJSONData, err := ioutil.ReadFile(devBiomeJSONPath)
		if err != nil {
			t.Error(err)
		} else {
			var cfg *biomeConfig
			if err := json.Unmarshal(devBiomeJSONData, &cfg); err != nil {
				t.Errorf("could not parse %s: %v", devBiomeJSONPath, err)
			} else {
				if cfg.ServeEnabled == nil || !*cfg.ServeEnabled || cfg.Launcher == nil || *cfg.Launcher != "local" {
					t.Errorf("%s content = %s; want serve_enabled = true, launcher = \"local\"", devBiomeJSONPath, devBiomeJSONData)
				}
			}
		}

		// .gitignore contains *.tfvars.
		const tfVarsIgnorePattern = "*.tfvars"
		gitignorePath := filepath.Join(projectDir, ".gitignore")
		gitignoreData, err := ioutil.ReadFile(gitignorePath)
		if err != nil {
			t.Errorf("read .gitignore: %+v", err)
		} else {
			if !containsLine(gitignoreData, tfVarsIgnorePattern) {
				t.Errorf("%s does not contain %q. Found:\n%s", gitignorePath, tfVarsIgnorePattern, gitignoreData)
			}
		}

		// .dockerignore contains *.tfvars.
		dockerignorePath := filepath.Join(projectDir, ".dockerignore")
		dockerignoreData, err := ioutil.ReadFile(dockerignorePath)
		if err != nil {
			t.Errorf("read .dockerignore: %+v", err)
		} else {
			if !containsLine(dockerignoreData, tfVarsIgnorePattern) {
				t.Errorf("%s does not contain %q. Found:\n%s", dockerignorePath, tfVarsIgnorePattern, dockerignoreData)
			}
		}

		// Running `go build` at top of project should succeed and produce a binary.
		goBuild := exec.Command("go", "build")
		goBuild.Dir = projectDir
		if output, err := goBuild.CombinedOutput(); err != nil {
			t.Errorf("go build returned error: %v. Output:\n%s", err, output)
		} else if _, err := os.Stat(filepath.Join(projectDir, exeName(projectName))); err != nil {
			t.Errorf("could not stat built binary: %v", err)
		}
	})
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func containsLine(data []byte, want string) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Equal(line, []byte(want)) {
			return true
		}
	}
	return false
}

// newTestProject creates a temporary project using "gocdk init" and
// returns a pctx with workdir set to the project directory, and a cleanup
// function.
func newTestProject(ctx context.Context) (*processContext, func(), error) {
	dir, err := ioutil.TempDir("", testTempDirPrefix)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		os.RemoveAll(dir)
	}
	pctx := &processContext{
		workdir: dir,
		env:     os.Environ(),
		stdout:  ioutil.Discard,
		stderr:  ioutil.Discard,
	}
	if err := run(ctx, pctx, []string{"init", "-m", "example.com/test", "--allow-existing-dir", dir}); err != nil {
		cleanup()
		return nil, nil, err
	}
	return pctx, cleanup, nil
}
