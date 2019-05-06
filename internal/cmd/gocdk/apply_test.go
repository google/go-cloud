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
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestApply(t *testing.T) {
	// TODO(#1809): test cases
	// didn't supply biome name to command
	// named biome doesn't exist
	// no terraform file
	// terraform not initialized *
	// terraform never previously applied
	// terraform apply repeat

	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not found:", err)
	}

	t.Run("TerraformNotInitialized", func(t *testing.T) {
		dir, cleanup, err := newTestModule()
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		const biomeName = "dev"
		const wantGreeting = "HALLO WORLD"
		if err := newTestBiome(dir, biomeName, wantGreeting, new(biomeConfig)); err != nil {
			t.Fatal(err)
		}
		pctx := &processContext{
			workdir: dir,
			stdout:  ioutil.Discard,
			stderr:  ioutil.Discard,
		}
		ctx := context.Background()

		// Call the main package run function as if 'apply' and biomeName were passed
		// on the command line. As part of this, ensureTerraformInit is called to check
		// that terraform has been properly initialized before running 'terraform apply'.
		if err := run(ctx, pctx, []string{"apply", biomeName}, new(bool)); err != nil {
			t.Errorf("run error: %+v", err)
		}

		// After a successful terraform apply, 'terraform output' should return the greeting
		// we configured. Terraform output fails if 'terraform init' was not called.
		// It also fails if 'terraform apply' has never been run, as there will be no
		// terraform state file (terraform.tfstate).
		outputs, err := tfReadOutput(ctx, findBiomeDir(dir, biomeName), os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		if got := outputs["greeting"].stringValue(); got != wantGreeting {
			t.Errorf("greeting = %q; want %q", got, wantGreeting)
		}
	})
}

func newTestBiome(dir, name, tfGreeting string, cfg *biomeConfig) error {
	biomeDir := filepath.Join(dir, "biomes", name)
	if err := os.MkdirAll(biomeDir, 0777); err != nil {
		return err
	}
	terraformSource := `output "greeting" {
	value = ` + strconv.Quote(tfGreeting) + `
}
provider "random" {
}`

	err := ioutil.WriteFile(filepath.Join(biomeDir, "main.tf"), []byte(terraformSource), 0666)
	if err != nil {
		return err
	}
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(biomeDir, "biome.json"), cfgData, 0666)
	if err != nil {
		return err
	}
	return nil
}
