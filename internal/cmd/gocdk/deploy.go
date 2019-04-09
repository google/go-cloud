package main

import (
	"context"
)

func deploy(ctx context.Context, pctx *processContext, args []string) error {
	if len(args) != 1 {
		return usagef("usage: gocdk deploy BIOME")
	}
	biome := args[0]
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
