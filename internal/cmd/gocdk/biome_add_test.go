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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBiomeAdd(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dir, cleanup, err := newTestModule()
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Join(dir, "biomes"), 0777)
		defer cleanup()

		ctx := context.Background()
		pctx := &processContext{
			workdir: dir,
			stdin:   strings.NewReader(""),
			stdout:  ioutil.Discard,
			stderr:  ioutil.Discard,
		}
		const newBiome = "foo"
		want := &biomeConfig{
			ServeEnabled: configBool(false),
			Launcher:     configString("cloudrun"),
		}

		if err := biomeAdd(ctx, pctx, []string{"add", newBiome}); err != nil {
			t.Fatal(err)
		}
		got, err := readBiomeConfig(dir, newBiome)
		if err != nil {
			t.Fatalf("biomeAdd: %+v", err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("biomeAdd diff (-want +got):\n%s", diff)
		}
	})
}
