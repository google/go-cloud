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
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/xerrors"
)

func TestReadBiomeConfig(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		dir, cleanup, err := newTestModule()
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		const biome = "foo"
		want := &biomeConfig{
			ServeEnabled: configBool(true),
			Launcher:     configString("rocket"),
		}
		if err := newTestBiome(dir, biome, "ohai", want); err != nil {
			t.Fatal(err)
		}

		got, err := readBiomeConfig(dir, biome)
		if err != nil {
			t.Fatalf("readBiomeConfig(%q, %q): %+v", dir, biome, err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("readBiomeConfig(%q, %q) diff (-want +got):\n%s", dir, biome, diff)
		}
	})
	t.Run("DirNotExist", func(t *testing.T) {
		dir, cleanup, err := newTestModule()
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		_, err = readBiomeConfig(dir, "dev")
		if !xerrors.As(err, new(*biomeNotFoundError)) {
			t.Errorf("readBiomeConfig(%q, \"dev\") error =\n%+v\n; want biome not found error", dir, err)
		}
	})
}

func configBool(b bool) *bool {
	return &b
}

func configString(s string) *string {
	return &s
}
