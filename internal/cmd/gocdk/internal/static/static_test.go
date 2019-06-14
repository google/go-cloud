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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
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
		"versions.tf",
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
		Description string
		Opts        *static.Options
		Action      *static.Action
		Filename    string
		// If not empty, Filename will created with this content
		Existing string

		WantErr bool
		// The test verifies that Filename has this content after executing Action,
		// even if WantErr is true.
		Want string
	}{
		{
			Description: "write a new file",
			Action: &static.Action{
				SourceContent: []byte("foo"),
				DestRelPath:   "foo.txt",
			},
			Filename: "foo.txt",
			Want:     "foo",
		},
		{
			Description: "preserve template syntax",
			Action: &static.Action{
				SourceContent: []byte("bar {{ . }}"),
				DestRelPath:   "bar.txt",
			},
			Filename: "bar.txt",
			Want:     "bar {{ . }}",
		},
		{
			Description: "using a template",
			Action: &static.Action{
				SourceContent: []byte("foo {{ . }} bar"),
				TemplateData:  "somedata",
				DestRelPath:   "foo.txt",
			},
			Filename: "foo.txt",
			Want:     "foo somedata bar",
		},
		{
			Description: "write a new file that exists -> error",
			Action: &static.Action{
				SourceContent: []byte("foo"),
				DestRelPath:   "foo.txt",
			},
			Filename: "foo.txt",
			Existing: "foo",
			WantErr:  true,
			Want:     "foo",
		},
		{
			Description: "write a new file that exists using force",
			Opts:        &static.Options{Force: true},
			Action: &static.Action{
				SourceContent: []byte("bar"),
				DestRelPath:   "foo.txt",
			},
			Filename: "foo.txt",
			Existing: "foo",
			Want:     "bar",
		},
		{
			Description: "DestExists is true but it no existing file -> error",
			Action: &static.Action{
				SourceContent: []byte("foo"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
			},
			Filename: "foo.txt",
			WantErr:  true,
		},
		{
			Description: "append",
			Action: &static.Action{
				SourceContent: []byte("foo\n"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
			},
			Filename: "foo.txt",
			Existing: "aaa\nbbb\n",
			Want:     "aaa\nbbb\nfoo\n",
		},
		{
			Description: "append is skipped due to default SkipMarker",
			Action: &static.Action{
				SourceContent: []byte("foo=bar\n"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
			},
			Filename: "foo.txt",
			Existing: "aaa\nfoo=bar\nbbb\n",
			Want:     "aaa\nfoo=bar\nbbb\n",
		},
		{
			Description: "append is skipped due to custom SkipMarker",
			Action: &static.Action{
				SourceContent: []byte("foo=bar\n"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
				SkipMarker:    "foo=",
			},
			Filename: "foo.txt",
			Existing: "aaa\nfoo=baz\nbbb\n",
			Want:     "aaa\nfoo=baz\nbbb\n",
		},
		{
			Description: "append is not skipped due to unmatched SkipMarker",
			Action: &static.Action{
				SourceContent: []byte("foo\n"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
				SkipMarker:    "bar",
			},
			Filename: "foo.txt",
			Existing: "aaa\nfoo\nbbb\n",
			Want:     "aaa\nfoo\nbbb\nfoo\n",
		},
		{
			Description: "insert into an existing file",
			Action: &static.Action{
				SourceContent: []byte("foo\n"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
				InsertMarker:  "marker",
			},
			Filename: "foo.txt",
			Existing: "aaa\nmarker\nccc\n",
			Want:     "aaa\nmarker\nfoo\nccc\n",
		},
		{
			Description: "insert marker not found",
			Action: &static.Action{
				SourceContent: []byte("foo\n"),
				DestRelPath:   "foo.txt",
				DestExists:    true,
				InsertMarker:  "marker",
			},
			Filename: "foo.txt",
			Existing: "aaa\nbbb\nccc\n",
			WantErr:  true,
			Want:     "aaa\nbbb\nccc\n",
		},
	}

	for _, test := range tests {
		t.Run(test.Description, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "static-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)

			destPath := filepath.Join(dir, test.Filename)

			// Write an existing file if needed.
			if test.Existing != "" {
				if err := ioutil.WriteFile(destPath, []byte(test.Existing), 0666); err != nil {
					t.Fatal(err)
				}
			}

			// Execute the action.
			err = static.Do(dir, test.Opts, test.Action)
			if (err != nil) != test.WantErr {
				t.Fatalf("got err %v, want err? %v", err, test.WantErr)
			}
			// If ReadFile fails, the file doesn't exist; treat this as no expected
			// content.
			got, _ := ioutil.ReadFile(destPath)
			if diff := cmp.Diff(string(got), test.Want); diff != "" {
				t.Error(diff)
			}
		})
	}
}
