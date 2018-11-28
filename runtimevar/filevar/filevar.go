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
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"

	"github.com/fsnotify/fsnotify"
)

// New constructs a runtimevar.Variable object with this package as the driver
// implementation.  The decoder argument allows users to dictate the decoding function to parse the
// file as well as the type to unmarshal into.
func New(file string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	w, err := newWatcher(file, decoder, opts)
	if err != nil {
		return nil, err
	}
	return runtimevar.New(w), nil
}

func newWatcher(file string, decoder *runtimevar.Decoder, opts *Options) (*watcher, error) {
	if opts == nil {
		opts = &Options{}
	}

	// Use absolute file path.
	file, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}

	// Construct a fsnotify.Watcher.
	notifier, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Create a ctx for the background goroutine that does all of the reading.
	// The cancel function will be used to shut it down during Close, with the
	// result being passed back via closeCh.
	ctx, cancel := context.WithCancel(context.Background())
	w := &watcher{
		// See struct comments for why it's buffered.
		ch:       make(chan *state, 1),
		closeCh:  make(chan error),
		shutdown: cancel,
	}
	go w.watch(ctx, notifier, file, decoder, driver.WaitDuration(opts.WaitDuration))
	return w, nil
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
	// The background goroutine writes new *state values to ch.
	// It is buffered so that the background goroutine can write without
	// blocking; it always drains the buffer before writing so that the latest
	// write is buffered. If writes could block, the background goroutine could be
	// blocked indefinitely from reading fsnotify events.
	ch chan *state
	// closeCh is used to return any errors from closing the notifier
	// back to watcher.Close.
	closeCh chan error
	// shutdown tells the background goroutine to exit.
	shutdown func()
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, _ driver.State) (driver.State, time.Duration) {
	select {
	case <-ctx.Done():
		return &state{err: ctx.Err()}, 0
	case cur := <-w.ch:
		return cur, 0
	}
}

// updateState checks to see if s and prev both represent the same error.
// If not, it drains any previous state buffered in w.ch, then writes s to it.
// It always return s.
func (w *watcher) updateState(s, prev *state) *state {
	if s.err != nil && prev != nil && prev.err != nil && (s.err == prev.err || s.err.Error() == prev.err.Error() || (os.IsNotExist(s.err) && os.IsNotExist(prev.err))) {
		// s represents the same error as prev.
		return s
	}
	// Drain any buffered value on ch; it is now stale.
	select {
	case <-w.ch:
	default:
	}
	// This write can't block, since we're the only writer, ch has a buffer
	// size of 1, and we just read anything that was buffered.
	w.ch <- s
	return s
}

// watch is run by a background goroutine.
// It watches file using notifier, and writes new states to w.ch.
// If it can't read or watch the file, it re-checks every wait.
// It exits when ctx is canceled, and writes any shutdown errors (or
// nil if there weren't any) to w.closeCh.
func (w *watcher) watch(ctx context.Context, notifier *fsnotify.Watcher, file string, decoder *runtimevar.Decoder, wait time.Duration) {
	var cur *state

	for {
		// If the current state is an error, pause between attempts
		// to avoid spin loops. In particular, this happens when the file
		// doesn't exist.
		if cur != nil && cur.err != nil {
			select {
			case <-ctx.Done():
				w.closeCh <- notifier.Close()
				return
			case <-time.After(wait):
			}
		}

		// Add the file to the notifier to be watched. It's fine to be
		// added multiple times, and fsnotifier is a bit flaky about when
		// it's needed during renames, so just always try.
		if err := notifier.Add(file); err != nil {
			// File probably does not exist. Try again later.
			cur = w.updateState(&state{err: err}, cur)
			continue
		}

		// Read the file.
		b, err := ioutil.ReadFile(file)
		if err != nil {
			// It's likely that the file was deleted.
			cur = w.updateState(&state{err: err}, cur)
			continue
		}

		// If it's a new value, decode and return it.
		if cur == nil || cur.err != nil || !bytes.Equal(cur.raw, b) {
			if val, err := decoder.Decode(b); err != nil {
				cur = w.updateState(&state{err: err}, cur)
			} else {
				cur = w.updateState(&state{val: val, updateTime: time.Now(), raw: b}, cur)
			}
		}

		// Block until notifier tells us something relevant changed.
		wait := true
		for wait {
			select {
			case <-ctx.Done():
				w.closeCh <- notifier.Close()
				return

			case event := <-notifier.Events:
				if event.Name != file {
					continue
				}
				// Ignore if not one of the following operations.
				if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
					continue
				}
				wait = false

			case err := <-notifier.Errors:
				cur = w.updateState(&state{err: err}, cur)
			}
		}
	}
}

// Options sets options.
type Options struct {
	// WaitDuration controls the frequency of retries after an error. For example,
	// if the file does not exist. Defaults to 30 seconds.
	WaitDuration time.Duration
}

// Close implements driver.WatchVariable.
func (w *watcher) Close() error {
	// Tell the background goroutine to shut down by canceling its ctx.
	w.shutdown()
	// Wait for it to return the result of closing the notifier.
	err := <-w.closeCh
	// Cleanup our channels.
	close(w.ch)
	close(w.closeCh)
	return err
}
