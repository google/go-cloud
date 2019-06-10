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
	"os"
	"os/exec"
	"strconv"
	"testing"

	"gocloud.dev/internal/cmd/gocdk/internal/static"
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
		ctx := context.Background()
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		const greeting = "HALLO WORLD"
		devBiomeDir := biomeDir(pctx.workdir, "dev")

		// Add a custom output variable to "main.tf".
		// We'll verify that we can read it down below.
		if err := static.Do(devBiomeDir, nil, &static.Action{
			SourceContent: []byte(`output "greeting" { value = ` + strconv.Quote(greeting) + `	}`),
			DestRelPath: "outputs.tf",
			DestExists:  true,
		}); err != nil {
			t.Fatal(err)
		}

		// Call the main package run function as if 'apply dev' were passed
		// on the command line. As part of this, ensureTerraformInit is called to check
		// that terraform has been properly initialized before running 'terraform apply'.
		if err := run(ctx, pctx, []string{"apply", "dev"}); err != nil {
			t.Errorf("run error: %+v", err)
		}

		// After a successful terraform apply, 'terraform output' should return the greeting
		// we configured. Terraform output fails if 'terraform init' was not called.
		// It also fails if 'terraform apply' has never been run, as there will be no
		// terraform state file (terraform.tfstate).
		outputs, err := tfReadOutput(ctx, devBiomeDir, os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		if got := outputs["greeting"].stringValue(); got != greeting {
			t.Errorf("greeting = %q; want %q", got, greeting)
		}
	})
}
