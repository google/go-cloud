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

// Package filevar provides a runtimevar driver implementation to read configurations and
// ability to detect changes and get updates on local configuration files.
//
// User constructs a runtimevar.Variable object using NewConfig given a locally-accessible file.
//
// User can update a configuration file using any commands (cp, mv) or tools/editors. This package
// does not guarantee read consistency since it does not have control over the writes. It is highly
// advisable to use this package only for local development or testing purposes and not in
// production applications/services.
//
// Known Issues:
//
// * On Mac OSX, if user copies an empty file into a configuration file, Watch will not be able to
// detect the change since event.Op is Chmod only.
//
// * Saving a configuration file in vim using :w will incur events Rename and Create. When the
// Rename event occurs, the file is temporarily removed and hence Watch will return error.  A
// follow-up Watch call will then detect the Create event.
package filevar

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"

	"github.com/fsnotify/fsnotify"
)

// defaultWait is the default amount of time for a watcher to reread the file.
// Change the docstring for NewVariable if this time is modified.
const defaultWait = 10 * time.Second

// NewVariable constructs a runtimevar.Variable object with this package as the driver
// implementation.  The decoder argument allows users to dictate the decoding function to parse the
// file as well as the type to unmarshal into.
// If WaitTime is not set the wait is set to 10 seconds.
func NewVariable(file string, decoder *runtimevar.Decoder, opts *WatchOptions) (*runtimevar.Variable, error) {
	if opts == nil {
		opts = &WatchOptions{}
	}
	waitTime := opts.WaitTime
	switch {
	case waitTime == 0:
		waitTime = defaultWait
	case waitTime < 0:
		return nil, fmt.Errorf("cannot have negative WaitTime option value: %v", waitTime)
	}

	// Use absolute file path.
	file, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}

	// Construct a fsnotify.Watcher but do not start watching yet. Make this call right before
	// returning a watcher to avoid having to close the fsnotify Watcher if there are more errors.
	notifier, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return runtimevar.New(&watcher{
		notifier: notifier,
		file:     file,
		decoder:  decoder,
		waitTime: waitTime,
	}), nil
}

// watcher implements driver.Watcher for configurations stored in files.
type watcher struct {
	notifier   *fsnotify.Watcher
	file       string
	decoder    *runtimevar.Decoder
	bytes      []byte
	isDeleted  bool
	waitTime   time.Duration
	updateTime time.Time
}

// WatchVariable blocks until the file changes, the Context's Done channel closes or an error occurs.  It
// will return an error if the configuration file is deleted, however, if it has previously returned
// an error due to missing configuration file, the next WatchVariable call will block until the file has
// been recreated.
func (w *watcher) WatchVariable(ctx context.Context) (driver.Variable, error) {
	zeroVar := driver.Variable{}
	// Check for Context cancellation first before proceeding.
	select {
	case <-ctx.Done():
		return zeroVar, ctx.Err()
	default:
		// Continue.
	}

	// Detect if there has been a changed first. If there are no changes, then proceed to add a
	// fsnotify watcher.
	wait := true
	for wait {
		sd := w.processFile()
		switch sd.state {
		case errorState:
			return zeroVar, sd.err

		case hasChangedState:
			return *sd.variable, nil

		case stillDeletedState:
			// Last known state is deleted, need to wait for file to show up before adding a watch.
			t := time.NewTimer(w.waitTime)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				return zeroVar, ctx.Err()
			}

		case noChangeState:
			wait = false
		}
	}

	// Start watching over the file and wait for events/errors.
	if err := w.notifier.Add(w.file); err != nil {
		if os.IsNotExist(err) {
			// File got deleted in between initial check above and adding a watch.
			w.isDeleted = true
			w.updateTime = time.Now().UTC()
		}
		return zeroVar, err
	}
	defer w.notifier.Remove(w.file)

	for {
		select {
		case <-ctx.Done():
			return zeroVar, ctx.Err()

		case event := <-w.notifier.Events:
			if event.Name != w.file {
				continue
			}
			// Ignore if not one of the following operations.
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			sd := w.processFile()
			switch sd.state {
			case errorState:
				return zeroVar, sd.err
			case hasChangedState:
				return *sd.variable, nil
			}
			// No changes, continue waiting.

		case err := <-w.notifier.Errors:
			return zeroVar, err
		}
	}
}

// WatchOptions allows the specification of various options to a watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of making an HTTP call and checking for
	// updates by the Watch method. The smaller the value, the higher the frequency
	// of making calls, which also means a faster rate of hitting the API quota.
	// If this option is not set or set to 0, it uses a default value of 10 seconds.
	WaitTime time.Duration
}

// watchState defines the different states during a watch for the Watch call to process.
type watchState int

const (
	noChangeState     watchState = iota
	hasChangedState              // Content has changed or file has been recovered from previous deletion.
	errorState                   // An error occurred either in processing or file has just been deleted.
	stillDeletedState            // File was previously marked deleted and is still deleted.
)

// stateData contains a watchState and corresponding data for it.
type stateData struct {
	state    watchState
	variable *driver.Variable // driver.Config object for hasChangedState.
	err      error            // Error object for hasErrorState.
}

// processFile reads the file and determines the watchState. Depending on the watchState, it may
// update the watcher's fields bytes, isDeleted and updateTime.
func (w *watcher) processFile() stateData {
	bytes, tm, err := readFile(w.file)
	if os.IsNotExist(err) {
		// File is deleted.
		if w.isDeleted {
			return stateData{state: stillDeletedState}
		}
		w.isDeleted = true
		w.updateTime = time.Now().UTC()
		return stateData{
			state: errorState,
			err:   err,
		}
	}
	if err != nil {
		return stateData{
			state: errorState,
			err:   err,
		}
	}
	// Change happens if it was previously deleted or content has changed.
	if w.isDeleted || bytesNotEqual(w.bytes, bytes) {
		w.bytes = bytes
		w.updateTime = tm
		w.isDeleted = false
		val, err := w.decoder.Decode(bytes)
		if err != nil {
			return stateData{
				state: errorState,
				err:   err,
			}
		}
		return stateData{
			state: hasChangedState,
			variable: &driver.Variable{
				Value:      val,
				UpdateTime: tm,
			},
		}
	}
	// No updates, no error.
	return stateData{state: noChangeState}
}

func readFile(file string) ([]byte, time.Time, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, time.Time{}, err
	}
	return b, time.Now().UTC(), nil
}

func bytesNotEqual(a []byte, b []byte) bool {
	n := len(a)
	if n != len(b) {
		return true
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// Close closes the fsnotify.Watcher.
func (w *watcher) Close() error {
	return w.notifier.Close()
}
