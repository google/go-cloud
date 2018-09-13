// Copyright 2018 The Go Cloud Authors
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

// Package filevar provides a runtimevar.Driver implementation that reads
// variables from local files.
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
	"bytes"
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

// defaultWait is the default value for WatchOptions.WaitTime.
const defaultWait = 30 * time.Second

// NewVariable constructs a runtimevar.Variable object with this package as the driver
// implementation.  The decoder argument allows users to dictate the decoding function to parse the
// file as well as the type to unmarshal into.
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

// state implements driver.State.
type state struct {
	val        interface{}
	updateTime time.Time
	raw        []byte
	err        error
}

func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// watcher implements driver.Watcher for configurations stored in files.
type watcher struct {
	notifier *fsnotify.Watcher
	file     string
	decoder  *runtimevar.Decoder
	waitTime time.Duration
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	s := &state{err: err}
	if prevS == nil {
		return s
	}
	prev := prevS.(*state)
	if prev.err == nil {
		// New error.
		return s
	}
	if err == prev.err {
		return nil
	}
	if os.IsNotExist(err) && os.IsNotExist(prev.err) {
		return nil
	}
	return s
}

func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	// Start watching over the file and wait for events/errors.
	// We can skip this if prev == nil, as we'll always immediately
	// return a value in that case.
	// We'll get a notifierErr if the file doesn't currently exist.
	var notifierErr error
	if prev != nil {
		notifierErr = w.notifier.Add(w.file)
		if notifierErr == nil {
			defer func() {
				_ = w.notifier.Remove(w.file)
			}()
		}
	}

	for {
		// Read the file.
		b, err := ioutil.ReadFile(w.file)
		if err != nil {
			// If the error hasn't changed, wait waitTime before trying again.
			// E.g., if the file doesn't exist.
			return errorState(err, prev), w.waitTime
		}

		// If it's a new value, decode and return it.
		if prev == nil || !bytes.Equal(prev.(*state).raw, b) {
			val, err := w.decoder.Decode(b)
			if err != nil {
				return errorState(err, prev), w.waitTime
			}
			return &state{val: val, updateTime: time.Now(), raw: b}, 0
		}

		// No change in variable value. Block until notifier tells us something
		// relevant changed.
		if notifierErr != nil {
			return errorState(err, prev), w.waitTime
		}
		wait := true
		for wait {
			select {
			case <-ctx.Done():
				return &state{err: ctx.Err()}, 0

			case event := <-w.notifier.Events:
				if event.Name != w.file {
					continue
				}
				// Ignore if not one of the following operations.
				if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
					continue
				}
				wait = false

			case err := <-w.notifier.Errors:
				return errorState(err, prev), w.waitTime
			}
		}
	}
}

// WatchOptions allows the specification of various options to a watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of retries after an error. For example,
	// if the file does not exist. Defaults to 30 seconds.
	WaitTime time.Duration
}

// Close closes the fsnotify.Watcher.
func (w *watcher) Close() error {
	return w.notifier.Close()
}
