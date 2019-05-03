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
	"flag"

	"golang.org/x/xerrors"
)

func deploy(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "deploy")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk deploy: %w", err)
	}

	if f.NArg() != 1 {
		return usagef("gocdk deploy BIOME")
	}
	biome := f.Arg(0)
	if err := build(ctx, pctx, nil); err != nil {
		return err
	}
	if err := apply(ctx, pctx, []string{biome}); err != nil {
		return err
	}
	if err := launch(ctx, pctx, []string{biome}); err != nil {
		return err
	}
	return nil
}
