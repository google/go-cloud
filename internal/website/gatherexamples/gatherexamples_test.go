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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestGather(t *testing.T) {
	tests := []struct {
		name   string
		module packagestest.Module
		want   map[string]string
	}{
		{
			name: "NoExamples",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\nfunc main() {}\n",
				},
			},
			want: map[string]string{},
		},
		{
			name: "EmptyExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func Example() {}`,
				},
			},
			want: map[string]string{
				"example.com/foo.Example": "",
			},
		},
		{
			name: "EmptyExampleFoo",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func ExampleFoo() {
}`,
				},
			},
			want: map[string]string{
				"example.com/foo.ExampleFoo": "",
			},
		},
		{
			name: "NoImportsExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

func Example() {
	// Unattached comment.

	// Outside inner block comment.
	panic("ohai")
	if false {
		// something
	}
	return
}`,
				},
			},
			want: map[string]string{
				"example.com/foo.Example": "// Unattached comment.\n\n" +
					"// Outside inner block comment.\n" +
					"panic(\"ohai\")\n" +
					"if false {\n\t// something\n}\n" +
					"return",
			},
		},
		{
			name: "OneImportExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "fmt"

func Example() {
	fmt.Println(42)
}`,
				},
			},
			want: map[string]string{
				"example.com/foo.Example": "import \"fmt\"\n\nfmt.Println(42)",
			},
		},
		{
			name: "TwoImportsExample",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "fmt"
import "math"

func Example() {
	fmt.Println(math.Pi)
}`,
				},
			},
			want: map[string]string{
				"example.com/foo.Example": "import (\n\t\"fmt\"\n\t\"math\"\n)\n\n" +
					"fmt.Println(math.Pi)",
			},
		},
		{
			name: "LogFatalToReturnErr",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "log"

func Example() {
	var err error
	if err != nil {
		log.Fatal(err)
	}
}`,
				},
			},
			want: map[string]string{
				"example.com/foo.Example": "import \"log\"\n\n" +
					"var err error\n" +
					"if err != nil {\n\treturn err\n}",
			},
		},
		{
			name: "IgnoreSections",
			module: packagestest.Module{
				Name: "example.com/foo",
				Files: map[string]interface{}{
					"foo.go": "package foo\n",
					"example_test.go": `package foo_test

import "context"

func Example() {
	// Variables set up elsewhere:
	ctx := context.Background()

	// do something

	// Ignore unused variables for example:
	_ = ctx
}`,
				},
			},
			want: map[string]string{
				"example.com/foo.Example": "import \"context\"\n\n// do something",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exported := packagestest.Export(t, packagestest.Modules, []packagestest.Module{test.module})
			defer exported.Cleanup()
			exported.Config.Mode = gatherLoadMode
			pkgs, err := packages.Load(exported.Config, "./...")
			if err != nil {
				t.Fatal(err)
			}

			got := gather(pkgs)
			if diff := cmp.Diff(test.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("gather(pkgs) diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatImports(t *testing.T) {
	tests := []struct {
		name         string
		usedPackages map[string]string
		want         string
	}{
		{
			name:         "Empty",
			usedPackages: nil,
			want:         "",
		},
		{
			name:         "One",
			usedPackages: map[string]string{"fmt": ""},
			want:         "import \"fmt\"",
		},
		{
			name: "Two",
			usedPackages: map[string]string{
				"fmt": "",
				"log": "",
			},
			want: "import (\n\t\"fmt\"\n\t\"log\"\n)",
		},
		{
			name: "Renamed",
			usedPackages: map[string]string{
				"fmt": "zzz",
				"log": "aaa",
			},
			want: "import (\n\tzzz \"fmt\"\n\taaa \"log\"\n)",
		},
		{
			name: "StdlibSeparateFromThirdParty",
			usedPackages: map[string]string{
				"context":                      "",
				"fmt":                          "",
				"log":                          "",
				"github.com/google/go-cmp/cmp": "",
				"gocloud.dev/blob":             "",
			},
			want: "import (\n" +
				"\t\"context\"\n" +
				"\t\"fmt\"\n" +
				"\t\"log\"\n" +
				"\n" +
				"\t\"github.com/google/go-cmp/cmp\"\n" +
				"\t\"gocloud.dev/blob\"\n" +
				")",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := formatImports(test.usedPackages)
			if got != test.want {
				t.Errorf("formatImports(%+v) =\n%s\n// want:\n%s", test.usedPackages, got, test.want)
			}
		})
	}
}
