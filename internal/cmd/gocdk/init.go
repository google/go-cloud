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
	"io/ioutil"
	"os"
	"path/filepath"

	"gocloud.dev/internal/cmd/gocdk/internal/templates"
	"golang.org/x/xerrors"
)

func init_(ctx context.Context, pctx *processContext, args []string) error {
	f := newFlagSet(pctx, "init")
	if err := f.Parse(args); xerrors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return usagef("gocdk init: %w", err)
	}

	if f.NArg() != 1 {
		return usagef("gocdk init PATH")
	}
	// TODO(light): allow an existing empty directory, for some definition of empty
	path := pctx.resolve(f.Arg(0))
	if _, err := os.Stat(path); err == nil {
		return xerrors.Errorf("gocdk init: %s already exists", path)
	} else if !os.IsNotExist(err) {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	if err := os.MkdirAll(path, 0777); err != nil {
		return xerrors.Errorf("gocdk init: %w", err)
	}

	for name, content := range templates.InitTemplates {
		fullPath := filepath.Join(path, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0777); err != nil {
			return xerrors.Errorf("gocdk init: %w", err)
		}
		if err := ioutil.WriteFile(fullPath, []byte(content), 0666); err != nil {
			return xerrors.Errorf("gocdk init: %w", err)
		}
	}
	return nil
}
