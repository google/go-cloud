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
	"errors"
	"flag"
	"os/exec"
	"testing"
	"time"

	"gocloud.dev/internal/testing/cmdtest"
)

var update = flag.Bool("update", false, "replace test file contents with output")

func Test(t *testing.T) {
	ts, err := cmdtest.Read(".")
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		out []byte
		err error
	}

	var (
		ctx    context.Context = context.Background()
		cancel func()
		outc   chan result
	)

	runtimevar := cmdtest.InProcessProgram("gocdk-runtimevar", func() int {
		return run(ctx)
	})
	ts.Commands["gocdk-runtimevar"] = runtimevar

	// "gocdk-runtimevar&" starts runtimevar in a separate goroutine.
	ts.Commands["gocdk-runtimevar&"] = func(args []string, infile string) ([]byte, error) {
		if cancel != nil {
			return nil, errors.New("go already in progress")
		}

		outc = make(chan result, 1)
		ctx, cancel = context.WithCancel(context.Background())
		go func() {
			out, err := runtimevar(args, infile)
			outc <- result{out, err}
		}()
		return nil, nil
	}

	// "stop" stops the command started with "gocdk-runtimevar&" and returns its output and error.
	ts.Commands["stop"] = func(args []string, _ string) ([]byte, error) {
		if cancel == nil {
			return nil, errors.New("no 'go' in progress")
		}
		cancel()
		res := <-outc
		if res.err == context.Canceled {
			res.err = nil
		}
		if ee, ok := res.err.(*exec.ExitError); ok && ee.ExitCode() == -1 { // killed by signal
			res.err = nil
		}

		ctx = context.Background()
		cancel = nil
		outc = nil

		return res.out, res.err
	}

	// "sleep <duration>" sleeps for <duration>
	ts.Commands["sleep"] = func(args []string, _ string) ([]byte, error) {
		if len(args) != 1 {
			return nil, errors.New("need exactly 1 argument")
		}
		dur, err := time.ParseDuration(args[0])
		if err != nil {
			return nil, err
		}
		time.Sleep(dur)
		return nil, nil
	}

	if err := ts.Run(*update); err != nil {
		t.Error(err)
	}
}
