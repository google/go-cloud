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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
)

func TestBiomeAdd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		const newBiome = "foo"
		if err := biomeAdd(ctx, pctx, newBiome, "local"); err != nil {
			t.Fatal(err)
		}

		// Ensure at least one file exists in the new biome with extension .tf.
		newBiomePath := filepath.Join(pctx.workdir, "biomes", newBiome)
		newBiomeContents, err := ioutil.ReadDir(newBiomePath)
		if err != nil {
			t.Error(err)
		} else {
			foundTF := false
			var foundNames []string
			for _, info := range newBiomeContents {
				foundNames = append(foundNames, info.Name())
				if filepath.Ext(info.Name()) == ".tf" {
					foundTF = true
				}
			}
			if !foundTF {
				t.Errorf("%s contains %v; want to contain at least one \".tf\" file", newBiomePath, foundNames)
			}
		}

		// Ensure that there is a biome.json file in the correct directory and
		// that it contains the correct settings for a local biome.
		want := &biomeConfig{
			ServeEnabled: configBool(true),
			Launcher:     configString("local"),
		}
		got, err := readBiomeConfig(pctx.workdir, newBiome)
		if err != nil {
			t.Fatalf("biomeAdd: %+v", err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("biomeAdd diff (-want +got):\n%s", diff)
		}
	})
}

func TestBiomeApply(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not found:", err)
	}

	ctx := context.Background()
	pctx, cleanup, err := newTestProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	const greeting = "HALLO WORLD"
	devBiomeDir, err := biomeDir(pctx.workdir, "dev")
	if err != nil {
		t.Fatal(err)
	}

	// Add a custom output variable to "main.tf".
	// We'll verify that we can read it down below.
	if err := static.Do(devBiomeDir, nil, &static.Action{
		SourceContent: []byte(`output "greeting" { value = ` + strconv.Quote(greeting) + `	}`),
		DestRelPath: "outputs.tf",
		DestExists:  true,
	}); err != nil {
		t.Fatal(err)
	}

	// Call the main package run function as if 'biome apply dev' were passed
	// on the command line.
	// This should run "terraform init", "terraform plan", and "terraform apply".
	if err := run(ctx, pctx, []string{"biome", "apply", "dev"}); err != nil {
		t.Fatalf("run error: %+v", err)
	}

	// After a successful "biome apply", "terraform output" should return the greeting
	// we configured. It will fail if "terraform init" was not called, or if
	// "terraform apply" wasn't , as there will be no terraform state file (terraform.tfstate).
	outputs, err := tfReadOutput(ctx, devBiomeDir, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if got := outputs["greeting"].stringValue(); got != greeting {
		t.Errorf("greeting = %q; want %q", got, greeting)
	}
}
