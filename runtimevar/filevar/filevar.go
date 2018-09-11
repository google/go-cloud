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
	notifier *fsnotify.Watcher
	file     string
	decoder  *runtimevar.Decoder
	waitTime time.Duration
}

func (w *watcher) WatchVariable(ctx context.Context, prevVersion interface{}, prevErr error) (*driver.Variable, interface{}, time.Duration, error) {

	// checkSameErr checks to see if err is the same as prevErr, and if so, returns
	// the "no change" signal with w.waitTime.
	// TODO(issue #412): Revisit as part of refactor to State interface.
	checkSameErr := func(err error) (*driver.Variable, interface{}, time.Duration, error) {
		if prevErr != nil {
			if (os.IsNotExist(err) && os.IsNotExist(prevErr)) || err.Error() == prevErr.Error() {
				return nil, nil, w.waitTime, nil
			}
		}
		return nil, nil, 0, err
	}

	// If we've got a value already, we're going to return whatever we get, so
	// there no need to set up a notifier. Otherwise, start watching the file
	// for events/errors.
	// We'll get a notifierErr if the file doesn't currently exist.
	// We may never use the notifier if we read the file below and detect a
	// change, but we must subscribe here to avoid race conditions.
	var notifierErr error
	if prevVersion != nil || prevErr != nil {
		notifierErr := w.notifier.Add(w.file)
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
			return checkSameErr(err)
		}

		// Check to see if the value is new. If so, return it.
		if prevVersion == nil || !bytes.Equal(prevVersion.([]byte), b) {
			val, err := w.decoder.Decode(b)
			if err != nil {
				return checkSameErr(err)
			}
			return &driver.Variable{Value: val, UpdateTime: time.Now()}, b, 0, nil
		}

		// No change in variable value. Block until notifier tells us something
		// relevant changed. If notifierErr is non-nil, the file is probably
		// deleted; we'll return the err above, waiting w.waitTime to check again.
		if notifierErr != nil {
			return checkSameErr(notifierErr)
		}
		wait := true
		for wait {
			select {
			case <-ctx.Done():
				return nil, nil, 0, ctx.Err()

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
				return checkSameErr(err)
			}
		}
	}
}

// WatchOptions allows the specification of various options to a watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of checking to see if a deleted file is
	// recreated.
	// Defaults to 10 seconds.
	WaitTime time.Duration
}

// Close closes the fsnotify.Watcher.
func (w *watcher) Close() error {
	return w.notifier.Close()
}
