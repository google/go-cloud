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
	"flag"
	"fmt"
	"io"
	"os"
	"testing"

	"gocloud.dev/internal/testing/cmdtest"
)

var update = flag.Bool("update", false, "replace test file contents with output")

func Test(t *testing.T) {
	ts, err := cmdtest.Read(".")
	if err != nil {
		t.Fatal(err)
	}
	ts.Commands["gocdk-blob"] = func(args []string, infile string) ([]byte, error) {
		return execInProcess(run, "gocdk-blob", args, infile)
	}
	ts.Verbose = true
	if err := ts.Run(*update); err != nil {
		t.Error(err)
	}
}

// execInProcess invokes f, which must behave like an actual main except that
// it returns an error code instead of calling os.Exit.
// Before calling f:
// - os.Args is set to the concatenation of name and args
// - if inputFile is non-empty, it is redirected to standard input
// - Standard output and standard error are redirected to a buffer, which is returned.
func execInProcess(f func() int, name string, args []string, inputFile string) ([]byte, error) {
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
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(buf, rOut)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(buf, rErr)
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
		err = fmt.Errorf("gocdk-blob failed with exit code %d", res)
	}
	return buf.Bytes(), err
}
