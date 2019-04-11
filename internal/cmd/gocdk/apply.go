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
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"
)

func apply(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "apply")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk apply: %w", err)
	}

	if f.NArg() != 1 {
		return usagef("gocdk apply BIOME")
	}
	biome := f.Arg(0)
	moduleRoot, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		return xerrors.Errorf("apply %s: %w", biome, err)
	}
	// TODO(cla): extract into helper method
	biomePath := filepath.Join(moduleRoot, "biomes", biome)

	// TODO(cla): take over steps (plan, confirm, apply) so we can
	// dictate the messaging and errors. We should visually differentiate
	// when we insert verbiage on top of terraform.
	c := exec.CommandContext(ctx, "terraform", "apply")
	c.Dir = biomePath
	c.Stdout = pctx.stdout
	c.Stderr = pctx.stderr
	if err := c.Run(); err != nil {
		return xerrors.Errorf("apply %s: %w", biome, err)
	}
	return nil
}
