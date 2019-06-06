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
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var record = flag.Bool("record", false, "true to record the desired output for record/replay testcases")

func TestProcessContextModuleRoot(t *testing.T) {
	ctx := context.Background()
	t.Run("SameDirAsModule", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		got, err := pctx.ModuleRoot(ctx)
		if got != pctx.workdir || err != nil {
			t.Errorf("got %q/%v want %q/<nil>", got, err, pctx.workdir)
		}
	})
	t.Run("NoBiomesDir", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := os.RemoveAll(biomesRootDir(pctx.workdir)); err != nil {
			t.Fatal(err)
		}

		if _, err = pctx.ModuleRoot(ctx); err == nil {
			t.Errorf("got nil error, want non-nil error due to missing biomes/ dir")
		}
	})
	t.Run("NoModFile", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := os.Remove(filepath.Join(pctx.workdir, "go.mod")); err != nil {
			t.Fatal(err)
		}

		if _, err = pctx.ModuleRoot(ctx); err == nil {
			t.Errorf("got nil error, want non-nil error due to missing go.mod")
		}
	})
	t.Run("ParentDirectory", func(t *testing.T) {
		pctx, cleanup, err := newTestProject(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		rootdir := pctx.workdir
		subdir := filepath.Join(rootdir, "subdir")
		if err := os.Mkdir(subdir, 0777); err != nil {
			t.Fatal(err)
		}
		pctx.workdir = subdir

		got, err := pctx.ModuleRoot(ctx)
		if got != rootdir || err != nil {
			t.Errorf("got %q/%v want %q/<nil>", got, err, rootdir)
		}
	})
}

// A record/replay testcase.
type testCase struct {
	// The directory where the testcase was found, e.g. testdata/recordreplay/Foo.
	dir string

	// The list of commands to execute.
	//
	// Empty lines and lines starting with "#" are ignored.
	//
	// By default, commands are expected to succeed, and the test will fail
	// otherwise. However, commands that are expected to fail can be marked
	// with a " --> FAIL" suffix.
	//
	// The following commands are supported:
	//  - gocdk: Executed through run().
	//  - cd: Takes exactly 1 argument, which cannot have "/" in it.
	//        Changes the working directory.
	//  - ls: With no arguments, recursively lists the files in the current
	//        working directory. With one argument, which cannot have "/" in it,
	//        recursively lists the file or directory named in the argument.
	commands []string

	// The desired success/failure for each command in commands.
	wantFail []bool

	// The desired STDOUT and STDERR (merged).
	//
	// In addition to the actual command output, the executed commands are written
	// to both, prefixed with "$ ", and a blank line is inserted after each
	// command. For example, the commands "ls" in a directory with a single file,
	// foo.txt, would result in:
	// $ ls
	// foo.txt
	//
	want []byte
}

// newTestCase reads a test case from dir.
//
// The directory structure is:
//
//	commands.txt: Commands to execute, one per line.
//  out.txt: Expected STDOUT/STDERR output.
//
//  See the testCase struct docstring for more info on each of the above.
func newTestCase(dir string, record bool) (*testCase, error) {
	const failMarker = " --> FAIL"

	name := filepath.Base(dir)
	tc := &testCase{dir: dir}

	commandsFile := filepath.Join(dir, "commands.txt")
	commandsBytes, err := ioutil.ReadFile(commandsFile)
	if err != nil {
		return nil, fmt.Errorf("load test case %s: %v", name, err)
	}
	for _, line := range strings.Split(string(commandsBytes), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		wantFail := false
		if strings.HasSuffix(line, failMarker) {
			line = strings.TrimSuffix(line, failMarker)
			wantFail = true
		}
		tc.commands = append(tc.commands, line)
		tc.wantFail = append(tc.wantFail, wantFail)
	}

	if !record {
		tc.want, err = ioutil.ReadFile(filepath.Join(dir, "out.txt"))
		if err != nil {
			return nil, err
		}
	}
	return tc, nil
}

func TestRecordReplay(t *testing.T) {
	const testRoot = "testdata/recordreplay"
	testcaseDirs, err := ioutil.ReadDir(testRoot)
	if err != nil {
		t.Fatal(err)
	}

	testcases := make([]*testCase, 0, len(testcaseDirs))
	for _, dir := range testcaseDirs {
		testcase, err := newTestCase(filepath.Join(testRoot, dir.Name()), *record)
		if err != nil {
			t.Fatal(err)
		}
		testcases = append(testcases, testcase)
	}

	ctx := context.Background()
	for _, tc := range testcases {
		tc := tc
		t.Run(filepath.Base(tc.dir), func(t *testing.T) {
			t.Parallel()

			rootDir, err := ioutil.TempDir("", testTempDirPrefix)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(rootDir)

			var out bytes.Buffer
			curDir := rootDir
			pctx := newProcessContext(curDir, strings.NewReader(""), &out, &out)
			pctx.env = append(pctx.env, "GOPATH="+rootDir)
			for i, command := range tc.commands {
				fmt.Fprintln(&out, "$", command)
				args := strings.Split(command, " ")
				cmd := args[0]
				args = args[1:]

				// We can add support for additional commands as needed; see
				// https://github.com/golang/go/blob/master/src/cmd/go/testdata/script/README
				// for inspiration.
				var cmdErr error
				switch cmd {
				case "gocdk":
					cmdErr = run(ctx, pctx, args)
				case "cd":
					if len(args) != 1 {
						t.Fatalf("command #%d: cd takes exactly 1 argument", i)
					}
					if strings.Contains(args[0], "/") {
						t.Fatalf("command #%d: argument to cd must be in the current directory (%q has a '/')", i, args[0])
					}
					curDir = filepath.Join(curDir, args[0])
					pctx.workdir = curDir
				case "ls":
					var arg string
					if len(args) == 1 {
						arg = args[0]
						if strings.Contains(arg, "/") {
							t.Fatalf("command #%d: argument to ls must be in the current directory (%q has a '/')", i, arg)
						}
					} else if len(args) > 1 {
						t.Fatalf("command #%d: ls takes 0-1 arguments (got %d)", i, len(args))
					}
					cmdErr = doList(curDir, &out, arg, "")
				default:
					t.Fatalf("unknown command #%d (%q)", i, command)
				}
				if cmdErr == nil && tc.wantFail[i] {
					t.Fatalf("command #%d (%q) succeeded, but it was expected to fail", i, command)
				}
				if cmdErr != nil && !tc.wantFail[i] {
					t.Fatalf("command #%d (%q) failed with %v", i, command, cmdErr)
				}
				fmt.Fprintln(&out)
			}

			got := scrub(rootDir, out.Bytes())
			if *record {
				if err := ioutil.WriteFile(filepath.Join(tc.dir, "out.txt"), got, 0666); err != nil {
					t.Fatalf("failed to record out.txt to testdata: %v", err)
				}
			} else {
				// Split to string lines to make diff output cleaner.
				gotLines := strings.Split(string(got), "\n")
				wantLines := strings.Split(string(tc.want), "\n")
				if diff := cmp.Diff(wantLines, gotLines); diff != "" {
					t.Errorf("out mismatch:\n%s", diff)
				}
			}
		})
	}
}

// doList recursively lists the files/directory in dir to out.
// If arg is not empty, it only lists arg.
func doList(dir string, out io.Writer, arg, indent string) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	// Sort case-insensitive. ioutil.ReadDir returns results in sorted order,
	// but it's not stable across platforms.
	sort.Slice(fileInfos, func(i, j int) bool {
		return strings.ToLower(fileInfos[i].Name()) < strings.ToLower(fileInfos[j].Name())
	})

	for _, fi := range fileInfos {
		if arg != "" && arg != fi.Name() {
			continue
		}
		if fi.IsDir() {
			fmt.Fprintf(out, "%s%s/\n", indent, fi.Name())
			if err := doList(filepath.Join(dir, fi.Name()), out, "", indent+"  "); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(out, "%s%s\n", indent, fi.Name())
		}
	}
	return nil
}

// scrub removes dynamic content from recorded files.
func scrub(rootDir string, b []byte) []byte {
	const scrubbedRootDir = "[ROOTDIR]"
	rootDirWithSeparator := rootDir + string(filepath.Separator)
	scrubbedRootDirWithSeparator := scrubbedRootDir + string(filepath.Separator)
	b = bytes.Replace(b, []byte(rootDirWithSeparator), []byte(scrubbedRootDirWithSeparator), -1)
	b = bytes.Replace(b, []byte(rootDir), []byte(scrubbedRootDir), -1)
	return b
}
