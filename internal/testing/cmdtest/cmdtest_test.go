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

package cmdtest

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var once sync.Once

func setup() {
	// Build echo-stdin, the little program needed to test input redirection.
	if err := exec.Command("go", "build", "testdata/echo-stdin.go").Run(); err != nil {
		log.Fatal(err)
	}
}

// echoStdin contains the same code as the main function of
// testdata/echo-stdin.go, except that it returns the exit code instead of
// calling os.Exit. It is for testing InProcessProgram.
func echoStdin() int {
	fmt.Println("Here is stdin:")
	_, err := io.Copy(os.Stdout, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed: %v\n", err)
		return 1
	}
	return 0
}

func TestMain(m *testing.M) {
	ret := m.Run()
	// Clean up the echo-stdin binary if we can. (No big deal if we can't.)
	cwd, err := os.Getwd()
	if err == nil {
		name := "echo-stdin"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		_ = os.Remove(filepath.Join(cwd, name))
	}
	os.Exit(ret)
}

func TestRead(t *testing.T) {
	got, err := Read("testdata/read")
	if err != nil {
		t.Fatal(err)
	}
	got.Commands = nil
	got.files[0].suite = nil
	want := &TestSuite{
		files: []*testFile{
			{
				filename: filepath.Join("testdata", "read", "read.ct"),
				cases: []*testCase{
					{
						before: []string{
							"# A sample test file.",
							"",
							"#   Prefix stuff.",
							"",
						},
						startLine:  5,
						commands:   []string{"command arg1 arg2", "cmd2"},
						wantOutput: []string{"out1", "out2"},
					},
					{
						before:     []string{"", "# start of the next case"},
						startLine:  11,
						commands:   []string{"c3"},
						wantOutput: nil,
					},
					{
						before:     []string{"", "# start of the third", ""},
						startLine:  15,
						commands:   []string{"c4 --> FAIL"},
						wantOutput: []string{"out3"},
					},
				},
				suffix: []string{"", "", "# end"},
			},
		},
	}
	if diff := cmp.Diff(got, want, cmp.AllowUnexported(TestSuite{}, testFile{}, testCase{})); diff != "" {
		t.Error(diff)
	}

}

func TestCompare(t *testing.T) {
	once.Do(setup)
	ts := mustReadTestSuite(t, "good")
	ts.Commands["echo-stdin"] = Program("echo-stdin")
	ts.Commands["echoStdin"] = InProcessProgram("echoStdin", echoStdin)
	ts.Run(t, false)

	// Test errors.
	// Since the output of cmp.Diff is unstable, we search for regexps we expect
	// to find there, rather than checking an exact match.
	ts = mustReadTestSuite(t, "bad")
	ts.Commands["echo-stdin"] = Program("echo-stdin")
	err := ts.compareReturningError()
	if err == nil {
		t.Fatal("got nil, want error")
	}
	got := err.Error()
	wants := []string{
		`testdata.bad.bad-output\.ct:2: got=-, want=+`,
		`testdata.bad.bad-output\.ct:6: got=-, want=+`,
		`testdata.bad.bad-fail-1\.ct:4: "echo" succeeded, but it was expected to fail`,
		`testdata.bad.bad-fail-2\.ct:4: "cd foo" failed with chdir`,
	}
	failed := false
	_ = failed
	for _, w := range wants {
		match, err := regexp.MatchString(w, got)
		if err != nil {
			t.Fatal(err)
		}
		if !match {
			t.Errorf(`output does not match "%s"`, w)
			failed = true
		}
	}
	if failed {
		// Log full output to aid debugging.
		t.Logf("output:\n%s", got)
	}
}

func TestExpandVariables(t *testing.T) {
	lookup := func(name string) (string, bool) {
		switch name {
		case "A":
			return "1", true
		case "B_C":
			return "234", true
		default:
			return "", false
		}
	}
	for _, test := range []struct {
		in, want string
	}{
		{"", ""},
		{"${A}", "1"},
		{"${A}${B_C}", "1234"},
		{" x${A}y  ${B_C}z ", " x1y  234z "},
		{" ${A${B_C}", " ${A234"},
	} {
		got, err := expandVariables(test.in, lookup)
		if err != nil {
			t.Errorf("%q: %v", test.in, err)
			continue
		}
		if got != test.want {
			t.Errorf("%q: got %q, want %q", test.in, got, test.want)
		}
	}

	// Unknown variable is an error.
	if _, err := expandVariables("x${C}y", lookup); err == nil {
		t.Error("got nil, want error")
	}
}

func TestUpdateToTemp(t *testing.T) {
	once.Do(setup)
	for _, dir := range []string{"good", "good-without-output"} {
		ts := mustReadTestSuite(t, dir)
		ts.Commands["echo-stdin"] = Program("echo-stdin")
		ts.Commands["echoStdin"] = InProcessProgram("echoStdin", echoStdin)
		fname, err := ts.files[0].updateToTemp()
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(fname)
		if diff := diffFiles(t, "testdata/good/good.ct", fname); diff != "" {
			t.Errorf("%s: %s", dir, diff)
		}
	}
}

func diffFiles(t *testing.T, gotFile, wantFile string) string {
	got, err := ioutil.ReadFile(gotFile)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ioutil.ReadFile(wantFile)
	if err != nil {
		t.Fatal(err)
	}
	return cmp.Diff(string(got), string(want))
}

func mustReadTestSuite(t *testing.T, dir string) *TestSuite {
	t.Helper()
	ts, err := Read(filepath.Join("testdata", dir))
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
