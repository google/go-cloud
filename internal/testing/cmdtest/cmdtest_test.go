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
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMain(m *testing.M) {
	// Build the little program needed to test input redirection.
	if err := exec.Command("go", "build", "testdata/echo-stdin.go").Run(); err != nil {
		log.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// Set PATH to the current directory so test files can find echo-stdin.
	os.Setenv("PATH", cwd)

	ret := m.Run()

	os.Remove(filepath.Join(cwd, "echo-stdin"))

	os.Exit(ret)
}

func TestRead(t *testing.T) {
	got, err := Read("testdata/read.ct")
	if err != nil {
		t.Fatal(err)
	}
	got.Commands = nil
	want := &TestFile{
		filename: "testdata/read.ct",
		cases: []*TestCase{
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
	}
	if diff := cmp.Diff(got, want, cmp.AllowUnexported(TestFile{}, TestCase{})); diff != "" {
		t.Error(diff)
	}

}

func TestCompare(t *testing.T) {
	tf := mustReadTestFile(t, "good")
	if diff := tf.Compare(); diff != "" {
		t.Errorf("got\n%s\nwant empty", diff)
	}

	// Test errors.
	// Since the output of cmp.Diff is unstable, we search for strings we expect
	// to find there, rather than checking an exact match.
	for _, test := range []struct {
		file  string
		wants []string // substrings that should be present
	}{
		{
			"bad-output",
			[]string{
				"testdata/bad-output.ct:2: got=-, want=+",
				"testdata/bad-output.ct:6: got=-, want=+",
			},
		},
		{
			"bad-fail-1",
			[]string{`testdata/bad-fail-1.ct:2: "echo" succeeded, but it was expected to fail`},
		},
		{
			"bad-fail-2",
			[]string{`testdata/bad-fail-2.ct:2: "cd foo" failed`},
		},
	} {
		tf := mustReadTestFile(t, test.file)
		got := tf.Compare()
		failed := false
		for _, w := range test.wants {
			if !strings.Contains(got, w) {
				t.Errorf("%s: output does not contain %q", test.file, w)
				failed = true
			}
		}
		if failed {
			t.Logf("output of %s:\n%s", test.file, got)
		}
	}
}

func TestExpand(t *testing.T) {
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
	tf := mustReadTestFile(t, "good")
	fname, err := tf.updateToTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fname)
	if diff := diffFiles(t, "testdata/good.ct", fname); diff != "" {
		t.Errorf("good.ct: %s", diff)
	}

	tf = mustReadTestFile(t, "good-without-output")
	fname, err = tf.updateToTemp()
	if err != nil {
		t.Fatal(err)
	}
	//defer os.Remove(fname)
	if diff := diffFiles(t, "testdata/good.ct", fname); diff != "" {
		fmt.Println(fname)
		t.Errorf("good-without-output.ct: %s", diff)
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

func mustReadTestFile(t *testing.T, basename string) *TestFile {
	t.Helper()
	tf, err := Read("testdata/" + basename + ".ct")
	if err != nil {
		t.Fatal(err)
	}
	return tf
}
