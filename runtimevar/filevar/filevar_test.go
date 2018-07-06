// Copyright 2018 Google LLC
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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cmp/cmp"
)

// Ensure that watcher implements driver.Watcher.
var _ driver.Watcher = &watcher{}

const waitTime = 5 * time.Millisecond

type file struct {
	name    string
	dir     string
	content string
}

func newFile() (*file, error) {
	const fileName = "app.json"
	const fileContent = `{"hello":"world"}`

	dir, err := ioutil.TempDir("", "filevar_test-")
	if err != nil {
		return nil, err
	}
	name := filepath.Join(dir, fileName)
	err = ioutil.WriteFile(name, []byte(fileContent), 0666)
	if err != nil {
		return nil, err
	}
	return &file{
		name:    name,
		dir:     dir,
		content: fileContent,
	}, nil
}

func (f *file) write(str string) error {
	f.content = str
	return ioutil.WriteFile(f.name, []byte(str), 0666)
}

func (f *file) delete() error {
	return os.Remove(f.name)
}

func setUp(t *testing.T) (*runtimevar.Variable, *file, func()) {
	t.Helper()
	f, err := newFile()
	if err != nil {
		t.Fatal(err)
	}

	variable, err := NewVariable(f.name, runtimevar.StringDecoder, &WatchOptions{WaitTime: waitTime})
	if err != nil {
		t.Fatalf("NewVariable returned error: %v", err)
	}

	return variable, f, func() {
		variable.Close()
		os.RemoveAll(f.dir)
	}
}

func TestFirstWatch(t *testing.T) {
	cfg, f, cleanUp := setUp(t)
	defer cleanUp()

	got, err := cfg.Watch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got.Value.(string), f.content); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}
}

func TestFirstWatchReturnsErrorIfFileNotFound(t *testing.T) {
	f, err := newFile()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.RemoveAll(f.dir)
	}()

	name := filepath.Base(f.name) + ".nonexist"
	cfg, err := NewVariable(filepath.Join(f.dir, name), runtimevar.StringDecoder, &WatchOptions{WaitTime: waitTime})
	if err != nil {
		t.Fatalf("NewVariable returned error: %v", err)
	}

	_, err = cfg.Watch(context.Background())
	if err == nil {
		t.Error("Variable.Watch returns nil, want error")
	}
}

func TestContextCancelledBeforeFirstWatch(t *testing.T) {
	cfg, _, cleanUp := setUp(t)
	defer cleanUp()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := cfg.Watch(ctx)
	if err == nil {
		t.Fatal("Variable.Watch returned nil error, expecting an error from cancelling")
	}
}

func TestContextCancelledInBetweenWatchCalls(t *testing.T) {
	cfg, _, cleanUp := setUp(t)
	defer cleanUp()

	ctx := context.Background()
	_, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = cfg.Watch(ctx)
	if err == nil {
		t.Fatal("Variable.Watch returned nil error, expecting an error from cancelling")
	}
}

func TestReturnsUpdatedSnapshot(t *testing.T) {
	cfg, f, cleanUp := setUp(t)
	defer cleanUp()

	ctx := context.Background()
	_, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	// Update the file.
	const content = `{"hello": "cloud"}`
	if err := f.write(content); err != nil {
		t.Fatal(err)
	}

	got, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	if diff := cmp.Diff(got.Value.(string), content); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}
}

func TestReturnsUpdatedSnapshotConcurrently(t *testing.T) {
	cfg, f, cleanUp := setUp(t)
	defer cleanUp()

	ctx := context.Background()
	_, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	const content = `{"hello": "cloud"}`
	// Update the file in a separate goroutine.
	written := make(chan struct{})
	defer func() {
		<-written
	}()
	go func() {
		defer close(written)
		if err := f.write(content); err != nil {
			t.Error("Writing:", err)
		}
	}()

	// Following will block until there is a change.
	for {
		got, err := cfg.Watch(ctx)
		if err != nil {
			t.Fatalf("Variable.Watch returned error: %v", err)
		}
		if got.Value.(string) == content {
			break
		}
		// Check for partial read or something entirely different.
		if !strings.HasPrefix(content, got.Value.(string)) {
			t.Errorf("Snapshot.Value = %q; want %q", got.Value.(string), content)
		}
	}
}

func TestDeletedAndReset(t *testing.T) {
	cfg, f, cleanUp := setUp(t)
	defer cleanUp()

	ctx := context.Background()
	prev, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	// Delete the file.
	if err := f.delete(); err != nil {
		t.Fatal(err)
	}

	// Expect deleted error.
	if _, err := cfg.Watch(ctx); err == nil {
		t.Fatalf("Variable.Watch returned nil, want error")
	}

	// Recreate file with new content and call Watch again.
	const content = `{"hello": "psp"}`
	if err := f.write(content); err != nil {
		t.Fatal(err)
	}

	got, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}
	if diff := cmp.Diff(got.Value.(string), content); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}
	if !got.UpdateTime.After(prev.UpdateTime) {
		t.Errorf("Snapshot.UpdateTime is less than or equal to previous value")
	}
}

func TestDeletedAndResetConcurrently(t *testing.T) {
	cfg, f, cleanUp := setUp(t)
	defer cleanUp()

	ctx := context.Background()
	prev, err := cfg.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	// Delete the file in a separate goroutine.
	deleted := make(chan struct{})
	defer func() { <-deleted }()
	go func() {
		defer close(deleted)
		if err := f.delete(); err != nil {
			t.Error("Deleting file:", err)
		}
	}()

	// Expect deleted error.
	if _, err := cfg.Watch(ctx); err == nil {
		t.Fatalf("Variable.Watch returned nil, want error")
	}

	// Recreate file with new content and call Watch again.
	const content = `{"hello": "psp"}`
	created := make(chan struct{})
	defer func() { <-created }()
	go func() {
		defer close(created)
		if err := f.write(content); err != nil {
			t.Error("Writing file:", err)
		}
	}()

	// Following will block until file gets recreated.
	for {
		got, err := cfg.Watch(ctx)
		if err != nil {
			t.Fatalf("Variable.Watch returned error: %v", err)
		}
		if !got.UpdateTime.After(prev.UpdateTime) {
			t.Errorf("Snapshot.UpdateTime is less than or equal to previous value")
		}
		if got.Value.(string) == content {
			break
		}
		// Check for partial read or something entirely different.
		if !strings.HasPrefix(content, got.Value.(string)) {
			t.Errorf("Snapshot.Value = %q; want %q", got.Value.(string), content)
		}
	}
}
