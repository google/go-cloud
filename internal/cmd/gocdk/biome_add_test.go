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
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
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
		if err := biomeAdd(ctx, pctx, []string{"add", newBiome}); err != nil {
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

		// Ensure that there is a biome.json file in teh correct directory and
		// that it contains the correct settings for a non-dev biome.
		want := &biomeConfig{
			ServeEnabled: configBool(false),
			Launcher:     configString("cloudrun"),
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
