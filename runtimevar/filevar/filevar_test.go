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

package filevar

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

type harness struct {
	dir    string
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	dir, err := ioutil.TempDir("", "filevar_test-")
	if err != nil {
		return nil, err
	}
	return &harness{
		dir:    dir,
		closer: func() { _ = os.RemoveAll(dir) },
	}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	// filevar uses a goroutine in the background that poll every WaitDuration if
	// the file is deleted. Make this fast for tests.
	return newWatcher(filepath.Join(h.dir, name), decoder, &Options{WaitDuration: 1 * time.Millisecond})
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	// Write to a temporary file and rename; otherwise,
	// Watch can read an empty file during the write.
	tmp, err := ioutil.TempFile(h.dir, "tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(val); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), filepath.Join(h.dir, name))
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.CreateVariable(ctx, name, val)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	path := filepath.Join(h.dir, name)
	return os.Remove(path)
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var ss string
	if s.As(&ss) {
		return errors.New("Snapshot.As expected to fail")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var ss string
	if v.ErrorAs(err, &ss) {
		return errors.New("runtimevar.ErrorAs expected to fail")
	}
	return nil
}

// Filevar-specific tests.

func TestNew(t *testing.T) {
	dir, err := ioutil.TempDir("", "filevar_test-")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		description string
		path        string
		decoder     *runtimevar.Decoder
		want        string
		wantErr     bool
	}{
		{
			description: "empty path results in error",
			decoder:     runtimevar.StringDecoder,
			wantErr:     true,
		},
		{
			description: "empty decoder results in error",
			path:        filepath.Join(dir, "foo.txt"),
			wantErr:     true,
		},
		{
			description: "basic path works",
			path:        filepath.Join(dir, "foo.txt"),
			decoder:     runtimevar.StringDecoder,
			want:        filepath.Join(dir, "foo.txt"),
		},
		{
			description: "path with extra relative dirs works and is cleaned up",
			path:        filepath.Join(dir, "bar/../foo.txt"),
			decoder:     runtimevar.StringDecoder,
			want:        filepath.Join(dir, "foo.txt"),
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			// Create driver impl.
			drv, err := newWatcher(test.path, test.decoder, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if drv != nil {
				if drv.path != test.want {
					t.Errorf("got %q want %q", drv.path, test.want)
				}
				drv.Close()
			}

			// Create portable type.
			w, err := New(test.path, test.decoder, nil)
			if (err != nil) != test.wantErr {
				t.Errorf("got err %v want error %v", err, test.wantErr)
			}
			if w != nil {
				w.Close()
			}
		})
	}
}
