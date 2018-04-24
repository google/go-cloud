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

// Package fileconfig provides a runtimeconfig driver implementation to read configurations and
// ability to detect changes and get updates on local configuration files.
//
// User constructs a runtimeconfig.Config object using NewConfig given a locally-accessible file.
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
package fileconfig

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/go-cloud/runtimeconfig"
	"github.com/google/go-cloud/runtimeconfig/driver"
)

// NewConfig constructs a runtimeconfig.Config object with this package as the driver
// implementation.  The decoder argument allows users to dictate the decoding function to parse the
// file as well as the type to unmarshal into.
func NewConfig(file string, decoder *runtimeconfig.Decoder) (*runtimeconfig.Config, error) {
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
	return runtimeconfig.New(&watcher{
		notifier: notifier,
		file:     file,
		decoder:  decoder,
	}), nil
}

// watcher implements driver.Watcher for configurations stored in files.
type watcher struct {
	notifier   *fsnotify.Watcher
	file       string
	decoder    *runtimeconfig.Decoder
	bytes      []byte
	isDeleted  bool
	updateTime time.Time
}

// Use of var instead of const to allow test to override value.
// TODO: Make this into a Watcher option.
var waitTime = 10 * time.Second

// Watch blocks until the file changes, the Context's Done channel closes or an error occurs.  It
// will return an error if the configuration file is deleted, however, if it has previously returned
// an error due to missing configuration file, the next Watch call will block until the file has
// been recreated.
func (w *watcher) Watch(ctx context.Context) (driver.Config, error) {
	zeroConfig := driver.Config{}
	// Check for Context cancellation first before proceeding.
	select {
	case <-ctx.Done():
		return zeroConfig, ctx.Err()
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
			return zeroConfig, sd.err

		case hasChangedState:
			return *sd.config, nil

		case stillDeletedState:
			// Last known state is deleted, need to wait for file to show up before adding a watch.
			t := time.NewTimer(waitTime)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				return zeroConfig, ctx.Err()
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
		return zeroConfig, err
	}
	defer w.notifier.Remove(w.file)

	for {
		select {
		case <-ctx.Done():
			return zeroConfig, ctx.Err()

		case event := <-w.notifier.Events:
			if event.Name != w.file {
				continue
			}
			// Ignore if not one of the following operations.
			if !(event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) != 0) {
				continue
			}
			sd := w.processFile()
			switch sd.state {
			case errorState:
				return zeroConfig, sd.err
			case hasChangedState:
				return *sd.config, nil
			}
			// No changes, continue waiting.

		case err := <-w.notifier.Errors:
			return zeroConfig, err
		}
	}
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
	state  watchState
	config *driver.Config // driver.Config object for hasChangedState.
	err    error          // Error object for hasErrorState.
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
			config: &driver.Config{
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
