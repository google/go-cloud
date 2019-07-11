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
	"flag"
	"testing"

	"gocloud.dev/internal/testing/cmdtest"
)

var update = flag.Bool("update", false, "replace test file contents with output")

func Test(t *testing.T) {
	ts, err := cmdtest.Read(".")
	if err != nil {
		t.Fatal(err)
	}
	ts.Commands["gocdk-secrets"] = cmdtest.InProcessProgram("gocdk-secrets", run)
	if err := ts.Run(*update); err != nil {
		t.Error(err)
	}
}
