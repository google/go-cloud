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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	// TODO(light): Test cases:
	// Happy path, verify that files we expect to exist do.
	// Wrong or missing arguments
	// Empty directory exists
	// Non-empty directory exists

	t.Run("CorrectPreconditions", func(t *testing.T) {
	})
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
	err = run(ctx, pctx, []string{"init", projectName}, new(bool))
	if err != nil {
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
		foundDockerfileComment := false
		const dockerfileComment = "# gocdk-image: " + projectName
		for _, line := range bytes.Split(dockerfileData, []byte("\n")) {
			if bytes.Equal(line, []byte(dockerfileComment)) {
				foundDockerfileComment = true
				break
			}
		}
		if !foundDockerfileComment {
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

	// Contains a biomes directory with "dev".
	// dev biome contains settings we expect.
	// Ensure that at least one file exists in dev biome with extension .tf.
	// Running `go build` at top of project should succeed and produce a binary.
	// (Maybe also start it and check it becomes healthy?)
	// .gitignore contains *.tfvars.
	// .dockerignore contains *.tfvars.
}
