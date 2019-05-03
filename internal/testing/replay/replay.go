// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package replay provides the ability to record and replay HTTP requests.
package replay // import "gocloud.dev/internal/testing/replay"

import (
	"path/filepath"
	"testing"

	"cloud.google.com/go/rpcreplay"
	"google.golang.org/grpc"
)

// NewGCPDialOptions return grpc.DialOptions that are to be appended to a GRPC
// dial request. These options allow a recorder/replayer to intercept RPCs and
// save RPCs to the file at filename, or read the RPCs from the file and return
// them. When recording is set to true, we're in recording mode; otherwise we're
// in replaying mode.
func NewGCPDialOptions(t *testing.T, recording bool, filename string) (opts []grpc.DialOption, done func()) {
	path := filepath.Join("testdata", filename)
	if recording {
		t.Logf("Recording into golden file %s", path)
		r, err := rpcreplay.NewRecorder(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		opts = r.DialOptions()
		done = func() {
			if err := r.Close(); err != nil {
				t.Errorf("unable to close recorder: %v", err)
			}
		}
		return opts, done
	}
	t.Logf("Replaying from golden file %s", path)
	r, err := rpcreplay.NewReplayer(path)
	if err != nil {
		t.Fatal(err)
	}
	// Uncomment for more verbose logging from the replayer.
	// r.SetLogFunc(t.Logf)
	opts = r.DialOptions()
	done = func() {
		if err := r.Close(); err != nil {
			t.Errorf("unable to close recorder: %v", err)
		}
	}
	return opts, done
}
