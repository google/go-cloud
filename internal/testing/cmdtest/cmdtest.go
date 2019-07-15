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

// The cmdtest package simplifies testing of command-line interfaces. It
// provides a simple, cross-platform, shell-like language to express command
// execution. It can compare actual output with the expected output, and can
// also update a file with new "golden" output that is deemed correct.
//
// Start using cmdtest by writing a test file with commands and expected output,
// giving it the extension ".ct". All test files in the same directory make up a
// test suite. See the TestSuite documentation for the syntax of test files.
//
// To test, first read the suite:
//
//    ts, err := cmdtest.Read("testdata")
//
// Then configure the resulting TestSuite by adding commands or enabling
// debugging features. Lastly, call TestSuite.Run with false to compare
// or true to update. Typically, this boolean will be the value of a flag:
//
//    var update = flag.Bool("update", false, "update test files with results")
//    ...
//    err := ts.Run(*update)
package cmdtest

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// A TestSuite contains a set of test files, each of which may contain multiple
// test cases. Use Read to build a TestSuite from all the test files in a
// directory. Then configure it and call Run.
//
// Format of a test file:
//
// Before the first line starting with a '$', empty lines and lines beginning with
// "#" are ignored.
//
// A sequence of consecutive lines starting with '$' begin a test case. These lines
// are commands to execute. See below for the valid commands.
//
// Lines following the '$' lines are command output (merged stdout and stderr).
// Output is always treated literally. After the command output there should be a
// blank line. Between that blank line and the next '$' line, empty lines and lines
// beginning with '#' are ignored. (Because of these rules, cmdtest cannot
// distinguish trailing blank lines in the output.)
//
// Syntax of a line beginning with '$': A sequence of space-separated words (no
// quoting is supported). The first word is the command, the rest are its args.
// If the next-to-last word is '<', the last word is interpreted as a file and
// becomes the standard input to the command. None of the built-in commands (see
// below) support input redirection, but commands defined with Program do.
//
// By default, commands are expected to succeed, and the test will fail
// otherwise. However, commands that are expected to fail can be marked
// with a " --> FAIL" suffix.
//
// The cases of a test file are executed in order, starting in a freshly
// created temporary directory.
//
// The built-in commands (initial contents of the Commands map) are:
//
//   cd DIR
//   cat FILE
//   mkdir DIR
//   setenv VAR VALUE
//   echo ARG1 ARG2 ...
//   echof FILE ARG1 ARG2 ...
//
// These all have their usual Unix shell meaning, except for echof, which writes its
// arguments to a file (output redirection is not supported). All file and directory
// arguments must refer to the current directory; that is, they cannot contain
// slashes.
//
// cmdtest does its own environment variable substitution, using the syntax
// "${VAR}". Test execution inherits the full environment of the test binary
// caller (typically, your shell). The environment variable ROOTDIR is set to
// the temporary directory created to run the test file.
type TestSuite struct {
	// If non-nil, this function is called for each test. It is passed the root
	// directory after it has been made the current directory.
	Setup func(string) error

	// The commands that can be executed (that is, whose names can occur as the
	// first word of a command line).
	Commands map[string]CommandFunc

	// If true, don't delete the temporary root directories for each test file,
	// and print out their names for debugging.
	KeepRootDirs bool

	files []*testFile
}

type testFile struct {
	suite    *TestSuite
	filename string // full filename of the test file
	cases    []*testCase
	suffix   []string // non-output lines after last case
}

type testCase struct {
	before    []string // lines before the commands
	startLine int      // line of first command
	// The list of commands to execute.
	commands []string

	// The stdout and stderr, merged and split into lines.
	gotOutput  []string // from execution
	wantOutput []string // from file
}

// CommandFunc is the signature of a command function. The function takes the
// subsequent words on the command line (so that arg[0] is the first argument),
// as well as the name of a file to use for input redirection. It returns the
// command's output.
type CommandFunc func(args []string, inputFile string) ([]byte, error)

// Read reads all the files in dir with extension ".ct" and returns a TestSuite
// containing them. See the TestSuite documentation for syntax.
func Read(dir string) (*TestSuite, error) {
	filenames, err := filepath.Glob(filepath.Join(dir, "*.ct"))
	if err != nil {
		return nil, err
	}
	ts := &TestSuite{
		Commands: map[string]CommandFunc{
			"cat":    fixedArgBuiltin(1, catCmd),
			"cd":     fixedArgBuiltin(1, cdCmd),
			"echo":   echoCmd,
			"echof":  echofCmd,
			"mkdir":  fixedArgBuiltin(1, mkdirCmd),
			"setenv": fixedArgBuiltin(2, setenvCmd),
		},
	}
	for _, fn := range filenames {
		tf, err := readFile(fn)
		if err != nil {
			return nil, err
		}
		tf.suite = ts
		ts.files = append(ts.files, tf)
	}
	return ts, nil
}

func readFile(filename string) (*testFile, error) {
	// parse states
	const (
		beforeFirstCommand = iota
		inCommands
		inOutput
	)

	tf := &testFile{
		filename: filename,
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var tc *testCase
	lineno := 0
	var prefix []string
	state := beforeFirstCommand
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		isCommand := strings.HasPrefix(line, "$")
		switch state {
		case beforeFirstCommand:
			if isCommand {
				tc = &testCase{startLine: lineno, before: prefix}
				tc.addCommandLine(line)
				state = inCommands
			} else {
				line = strings.TrimSpace(line)
				if line == "" || line[0] == '#' {
					prefix = append(prefix, line)
				} else {
					return nil, fmt.Errorf("%s:%d: bad line %q (should begin with '#')", filename, lineno, line)
				}
			}

		case inCommands:
			if isCommand {
				tc.addCommandLine(line)
			} else { // End of commands marks the start of the output.
				tc.wantOutput = append(tc.wantOutput, line)
				state = inOutput
			}

		case inOutput:
			if isCommand { // A command marks the end of the output.
				prefix = tf.addCase(tc)
				tc = &testCase{startLine: lineno, before: prefix}
				tc.addCommandLine(line)
				state = inCommands
			} else {
				tc.wantOutput = append(tc.wantOutput, line)
			}
		default:
			panic("bad state")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if tc != nil {
		tf.suffix = tf.addCase(tc)
	}
	return tf, nil
}

func (tc *testCase) addCommandLine(line string) {
	tc.commands = append(tc.commands, strings.TrimSpace(line[1:]))
}

// addCase first splits the collected output for tc into the actual command
// output, and a suffix consisting of blank lines and comments. It then adds tc
// to the cases of tf, and returns the suffix.
func (tf *testFile) addCase(tc *testCase) []string {
	// Trim the suffix of output that consists solely of blank lines and comments,
	// and return it.
	var i int
	for i = len(tc.wantOutput) - 1; i >= 0; i-- {
		if tc.wantOutput[i] != "" && tc.wantOutput[i][0] != '#' {
			break
		}
	}
	i++
	// i is the index of the first line to ignore.
	keep, suffix := tc.wantOutput[:i], tc.wantOutput[i:]
	if len(keep) == 0 {
		keep = nil
	}
	tc.wantOutput = keep
	tf.cases = append(tf.cases, tc)
	return suffix
}

// Run runs the commands in each file in the test suite. Each file runs in a
// separate subtest.
//
// If update is false, it compares their output with the output in the file,
// line by line.
//
// If update is true, it writes the output back to the file, overwriting the
// previous output.
//
// Before comparing/updating, occurrences of the root directory in the output
// are replaced by ${ROOTDIR}.
func (ts *TestSuite) Run(t *testing.T, update bool) {
	if update {
		ts.update(t)
	} else {
		ts.compare(t)
	}
}

// compare runs a subtest for each file in the test suite. See Run.
func (ts *TestSuite) compare(t *testing.T) {
	for _, tf := range ts.files {
		t.Run(strings.TrimSuffix(tf.filename, ".ct"), func(t *testing.T) {
			if s := tf.compare(t.Logf); s != "" {
				t.Error(s)
			}
		})
	}
}

var noopLogger = func(_ string, _ ...interface{}) {}

// compareReturningError is similar to compare, but it returns
// errors/differences in an error. It is used in tests for this package.
func (ts *TestSuite) compareReturningError() error {
	var ss []string
	for _, tf := range ts.files {
		if s := tf.compare(noopLogger); s != "" {
			ss = append(ss, s)
		}
	}
	if len(ss) > 0 {
		return errors.New(strings.Join(ss, ""))
	}
	return nil
}

func (tf *testFile) compare(log func(string, ...interface{})) string {
	if err := tf.execute(log); err != nil {
		return fmt.Sprintf("%v", err)
	}
	buf := new(bytes.Buffer)
	for _, c := range tf.cases {
		if diff := cmp.Diff(c.gotOutput, c.wantOutput); diff != "" {
			fmt.Fprintf(buf, "%s:%d: got=-, want=+\n", tf.filename, c.startLine)
			c.writeCommands(buf)
			fmt.Fprintf(buf, "%s\n", diff)
		}
	}
	return buf.String()
}

// update runs a subtest for each file in the test suite, updating their output.
// See Run.
func (ts *TestSuite) update(t *testing.T) {
	for _, tf := range ts.files {
		t.Run(strings.TrimSuffix(tf.filename, ".ct"), func(t *testing.T) {
			tmpfile, err := tf.updateToTemp()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(tmpfile, tf.filename); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// updateToTemp executes tf and writes the output to a temporary file.
// It returns the name of the temporary file.
func (tf *testFile) updateToTemp() (fname string, err error) {
	if err := tf.execute(noopLogger); err != nil {
		return "", err
	}

	f, err := ioutil.TempFile("", "cmdtest")
	if err != nil {
		return "", err
	}
	defer func() {
		err2 := f.Close()
		if err == nil {
			err = err2
		}
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	if err := tf.write(f); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func (tf *testFile) execute(log func(string, ...interface{})) error {
	rootDir, err := ioutil.TempDir("", "cmdtest")
	if err != nil {
		return fmt.Errorf("%s: %v", tf.filename, err)
	}
	if tf.suite.KeepRootDirs {
		fmt.Printf("%s: test root directory: %s\n", tf.filename, rootDir)
	} else {
		defer os.RemoveAll(rootDir)
	}

	if err := os.Setenv("ROOTDIR", rootDir); err != nil {
		return fmt.Errorf("%s: %v", tf.filename, err)
	}
	defer os.Unsetenv("ROOTDIR")
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%s: %v", tf.filename, err)
	}

	if err := os.Chdir(rootDir); err != nil {
		return fmt.Errorf("%s: %v", tf.filename, err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if tf.suite.Setup != nil {
		if err := tf.suite.Setup(rootDir); err != nil {
			return fmt.Errorf("%s: calling Setup: %v", tf.filename, err)
		}
	}
	for _, tc := range tf.cases {
		if err := tc.execute(tf.suite, log); err != nil {
			return fmt.Errorf("%s:%v", tf.filename, err) // no space after :, for line number
		}
	}
	return nil
}

// A fatal error stops a test.
type fatal struct{ error }

// Run the test case by executing the commands. The concatenated output from all commands
// is saved in tc.gotOutput.
// An error is returned if: a command that should succeed instead failed; a command that should
// fail instead succeeded; or a built-in command was called incorrectly.
func (tc *testCase) execute(ts *TestSuite, log func(string, ...interface{})) error {
	const failMarker = " --> FAIL"

	tc.gotOutput = nil
	var allout []byte
	var err error
	for i, cmd := range tc.commands {
		wantFail := false
		if strings.HasSuffix(cmd, failMarker) {
			cmd = strings.TrimSuffix(cmd, failMarker)
			wantFail = true
		}
		args := strings.Fields(cmd)
		for i := range args {
			args[i], err = expandVariables(args[i], os.LookupEnv)
			if err != nil {
				return err
			}
		}
		log("$ %s", strings.Join(args, " "))
		name := args[0]
		args = args[1:]
		var infile string
		if len(args) >= 2 && args[len(args)-2] == "<" {
			infile = args[len(args)-1]
			args = args[:len(args)-2]
		}
		f := ts.Commands[name]
		if f == nil {
			return fmt.Errorf("%d: no such command %q", tc.startLine+i, name)
		}
		out, err := f(args, infile)
		if _, ok := err.(fatal); ok {
			return fmt.Errorf("%d: command %q failed fatally with %v", tc.startLine+i, cmd, err)
		}
		if err == nil && wantFail {
			return fmt.Errorf("%d: %q succeeded, but it was expected to fail", tc.startLine+i, cmd)
		}
		if err != nil && !wantFail {
			return fmt.Errorf("%d: %q failed with %v. Output:\n%s", tc.startLine+i, cmd, err, out)
		}
		log("%s\n", string(out))
		allout = append(allout, out...)
	}
	if len(allout) > 0 {
		allout = scrub(os.Getenv("ROOTDIR"), allout) // use Getenv because Setup could change ROOTDIR
		// Remove final whitespace.
		s := strings.TrimRight(string(allout), " \t\n")
		tc.gotOutput = strings.Split(s, "\n")
	}
	return nil
}

// Program defines a command function that will run the executable at path using
// the exec.Command package and return its combined output. If path is relative,
// it is converted to an absolute path using the current directory at the time
// Program is called.
//
// In the unlikely event that Program cannot obtain the current directory, it
// panics.
func Program(path string) CommandFunc {
	abspath, err := filepath.Abs(path)
	if err != nil {
		panic(fmt.Sprintf("Program(%q): %v", path, err))
	}
	return func(args []string, inputFile string) ([]byte, error) {
		return execute(abspath, args, inputFile)
	}
}

// InProcessProgram defines a command function that will invoke f, which must
// behave like an actual main function except that it returns an error code
// instead of calling os.Exit.
// Before calling f:
// - os.Args is set to the concatenation of name and args.
// - If inputFile is non-empty, it is redirected to standard input.
// - Standard output and standard error are redirected to a buffer, which is
// returned.
func InProcessProgram(name string, f func() int) CommandFunc {
	return func(args []string, inputFile string) ([]byte, error) {
		origArgs := os.Args
		origOut := os.Stdout
		origErr := os.Stderr
		defer func() {
			os.Args = origArgs
			os.Stdout = origOut
			os.Stderr = origErr
		}()
		os.Args = append([]string{name}, args...)
		// Redirect stdout and stderr to pipes.
		rOut, wOut, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		rErr, wErr, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		os.Stdout = wOut
		os.Stderr = wErr
		// Copy both stdout and stderr to the same buffer.
		buf := &bytes.Buffer{}
		lw := &lockingWriter{w: buf}
		errc := make(chan error, 2)
		go func() {
			_, err := io.Copy(lw, rOut)
			errc <- err
		}()
		go func() {
			_, err := io.Copy(lw, rErr)
			errc <- err
		}()

		// Redirect stdin if needed.
		if inputFile != "" {
			f, err := os.Open(inputFile)
			if err != nil {
				return nil, err
			}
			defer f.Close()
			origIn := os.Stdin
			defer func() { os.Stdin = origIn }()
			os.Stdin = f
		}

		res := f()
		if err := wOut.Close(); err != nil {
			return nil, err
		}
		if err := wErr.Close(); err != nil {
			return nil, err
		}
		// Wait for pipe copying to finish.
		if err := <-errc; err != nil {
			return nil, err
		}
		if err := <-errc; err != nil {
			return nil, err
		}
		if res != 0 {
			err = fmt.Errorf("%s failed with exit code %d", name, res)
		}
		return buf.Bytes(), err
	}
}

// lockingWriter is an io.Writer whose Write method is safe for
// use by multiple goroutines.
type lockingWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockingWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(b)
}

// execute uses exec.Command to run the named program with the given args. The
// combined output is captured and returned. If infile is not empty, its contents
// become the command's standard input.
func execute(name string, args []string, infile string) ([]byte, error) {
	ecmd := exec.Command(name, args...)
	var errc chan error
	if infile != "" {
		f, err := os.Open(infile)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		ecmd.Stdin = f
	}
	out, err := ecmd.CombinedOutput()
	if err != nil {
		return out, err
	}
	if errc != nil {
		if err = <-errc; err != nil {
			return out, err
		}
	}
	return out, nil
}

var varRegexp = regexp.MustCompile(`\$\{([^${}]+)\}`)

// expandVariables replaces variable references in s with their values. A reference
// to a variable V looks like "${V}".
// lookup is called on a variable's name to find its value. Its second return value
// is false if the variable doesn't exist.
// expandVariables fails if s contains a reference to a non-existent variable.
//
// This function differs from os.Expand in two ways. First, it does not expand $var,
// only ${var}. The former is fragile. Second, an undefined variable results in an error,
// rather than expanding to some string. We want to fail if a variable is undefined.
func expandVariables(s string, lookup func(string) (string, bool)) (string, error) {
	var sb strings.Builder
	for {
		ixs := varRegexp.FindStringSubmatchIndex(s)
		if ixs == nil {
			sb.WriteString(s)
			return sb.String(), nil
		}
		varName := s[ixs[2]:ixs[3]]
		varVal, ok := lookup(varName)
		if !ok {
			return "", fmt.Errorf("variable %q not found", varName)
		}
		sb.WriteString(s[:ixs[0]])
		sb.WriteString(varVal)
		s = s[ixs[1]:]
	}
}

// scrub removes dynamic content from output.
func scrub(rootDir string, b []byte) []byte {
	const scrubbedRootDir = "${ROOTDIR}"
	rootDirWithSeparator := rootDir + string(filepath.Separator)
	scrubbedRootDirWithSeparator := scrubbedRootDir + "/"
	b = bytes.Replace(b, []byte(rootDirWithSeparator), []byte(scrubbedRootDirWithSeparator), -1)
	b = bytes.Replace(b, []byte(rootDir), []byte(scrubbedRootDir), -1)
	return b
}

func (tf *testFile) write(w io.Writer) error {
	for _, c := range tf.cases {
		if err := c.write(w); err != nil {
			return err
		}
	}
	return writeLines(w, tf.suffix)
}

func (tc *testCase) write(w io.Writer) error {
	if err := writeLines(w, tc.before); err != nil {
		return err
	}
	if err := tc.writeCommands(w); err != nil {
		return err
	}
	out := tc.gotOutput
	if out == nil {
		out = tc.wantOutput
	}
	return writeLines(w, out)
}

func (tc *testCase) writeCommands(w io.Writer) error {
	for _, c := range tc.commands {
		if _, err := fmt.Fprintf(w, "$ %s\n", c); err != nil {
			return err
		}
	}
	return nil
}

func writeLines(w io.Writer, lines []string) error {
	for _, l := range lines {
		if _, err := io.WriteString(w, l); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}

func fixedArgBuiltin(nargs int, f func([]string) ([]byte, error)) CommandFunc {
	return func(args []string, inputFile string) ([]byte, error) {
		if len(args) != nargs {
			return nil, fatal{fmt.Errorf("need exactly %d arguments", nargs)}
		}
		if inputFile != "" {
			return nil, fatal{errors.New("input redirection not supported")}
		}
		return f(args)
	}
}

// cd DIR
// change directory
func cdCmd(args []string) ([]byte, error) {
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return nil, os.Chdir(filepath.Join(cwd, args[0]))
}

// echo ARG1 ARG2 ...
// write args to stdout
//
// \n is added at the end of the input.
// Also, literal "\n" in the input will be replaced by \n.
func echoCmd(args []string, inputFile string) ([]byte, error) {
	if inputFile != "" {
		return nil, fatal{errors.New("input redirection not supported")}
	}
	s := strings.Join(args, " ")
	s = strings.Replace(s, "\\n", "\n", -1)
	s += "\n"
	return []byte(s), nil
}

// echof FILE ARG1 ARG2 ...
// write args to FILE
//
// \n is added at the end of the input.
// Also, literal "\n" in the input will be replaced by \n.
func echofCmd(args []string, inputFile string) ([]byte, error) {
	if len(args) < 1 {
		return nil, fatal{errors.New("need at least 1 argument")}
	}
	if inputFile != "" {
		return nil, fatal{errors.New("input redirection not supported")}
	}
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	s := strings.Join(args[1:], " ")
	s = strings.Replace(s, "\\n", "\n", -1)
	s += "\n"
	return nil, ioutil.WriteFile(args[0], []byte(s), 0600)
}

// cat FILE
// copy file to stdout
func catCmd(args []string) ([]byte, error) {
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	f, err := os.Open(args[0])
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, f)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// mkdir DIR
// create directory
func mkdirCmd(args []string) ([]byte, error) {
	if err := checkPath(args[0]); err != nil {
		return nil, err
	}
	return nil, os.Mkdir(args[0], 0700)
}

// setenv VAR VALUE
// set environment variable
func setenvCmd(args []string) ([]byte, error) {
	return nil, os.Setenv(args[0], args[1])
}

func checkPath(path string) error {
	if strings.ContainsRune(path, '/') || strings.ContainsRune(path, '\\') {
		return fatal{fmt.Errorf("argument must be in the current directory (%q has a '/')", path)}
	}
	return nil
}
