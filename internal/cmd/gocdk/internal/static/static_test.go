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

package static_test

import (
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/cmd/gocdk/internal/static"
)

func TestListFiles(t *testing.T) {
	// Test using the biome subdirectory, which seems the least likely to change
	// over time.
	want := []string{
		"biome.json",
		"main.tf",
		"outputs.tf",
		"secrets.auto.tfvars",
		"variables.tf",
	}

	// List the files under biome/. They should be returned without the biome/
	// prefix, as listed in want.
	got, err := static.ListFiles("/biome")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Error(diff)
	}

	// List the files under /. We should still see the biome/ files listed in
	// want, but this time they should have the "biome/" prefix.
	got, err = static.ListFiles("/")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range want {
		wantWithPrefix := path.Join("biome", w)
		found := false
		for _, g := range got {
			if g == wantWithPrefix {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("didn't find %q", wantWithPrefix)
		}
	}
}

func TestDo(t *testing.T) {
	tests := []struct {
	}{}

	for _, test := range tests {

	}
}
