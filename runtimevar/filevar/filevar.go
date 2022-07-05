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

// Package filevar provides a runtimevar implementation with variables
// backed by the filesystem. Use OpenVariable to construct a *runtimevar.Variable.
//
// Configuration files can be updated using any commands (cp, mv) or
// tools/editors. This package does not guarantee read consistency since
// it does not have control over the writes. For example, some kinds of
// updates might result in filevar temporarily receiving an error or an
// empty value.
//
// Known Issues:
//
// * On macOS, if an empty file is copied into a configuration file,
//
//	filevar will not detect the change.
//
// # URLs
//
// For runtimevar.OpenVariable, filevar registers for the scheme "file".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// filevar does not support any types for As.
package filevar // import "gocloud.dev/runtimevar/filevar"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, &URLOpener{})
}

// Scheme is the URL scheme filevar registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "file"

// URLOpener opens filevar URLs like "file:///path/to/config.json?decoder=json".
//
// The URL's host+path is used as the path to the file to watch.
// If os.PathSeparator != "/", any leading "/" from the path is dropped
// and remaining '/' characters are converted to os.PathSeparator.
//
// The following URL parameters are supported:
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//     runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//     See runtimevar.DecoderByName for supported values.
//   - wait: The frequency for retries after an error, in time.ParseDuration formats.
//     Defaults to 30s.
type URLOpener struct {
	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the options to pass to OpenVariable.
	Options Options
}

// OpenVariableURL opens the variable at the URL's path. See the package doc
// for more details.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(ctx, decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}
	opts := o.Options
	if s := q.Get("wait"); s != "" {
		q.Del("wait")
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("open variable %v: invalid wait %q: %v", u, s, err)
		}
		opts.WaitDuration = d
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}
	path := u.Path
	if os.PathSeparator != '/' {
		path = strings.TrimPrefix(path, "/")
	}
	return OpenVariable(filepath.FromSlash(path), decoder, &opts)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the frequency of retries after an error. For example,
	// if the file does not exist. Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the file at path.
// The file holds raw bytes; provide a decoder to decode the raw bytes into the
// appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(path string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	w, err := newWatcher(path, decoder, opts)
	if err != nil {
		return nil, err
	}
	return runtimevar.New(w), nil
}

func newWatcher(path string, decoder *runtimevar.Decoder, opts *Options) (*watcher, error) {
	if opts == nil {
		opts = &Options{}
	}
	if path == "" {
		return nil, errors.New("path is required")
	}
	if decoder == nil {
		return nil, errors.New("decoder is required")
	}

	// Use absolute file path.
	abspath, err := filepath.Abs(path)
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
		path: abspath,
		// See struct comments for why it's buffered.
		ch:       make(chan *state, 1),
		closeCh:  make(chan error),
		shutdown: cancel,
	}
	go w.watch(ctx, notifier, abspath, decoder, driver.WaitDuration(opts.WaitDuration))
	return w, nil
}

// errNotExist wraps an underlying error in cases where the file likely doesn't
// exist.
type errNotExist struct {
	err error
}

func (e *errNotExist) Error() string {
	return e.err.Error()
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

func (s *state) As(i interface{}) bool {
	return false
}

// watcher implements driver.Watcher for configurations stored in files.
type watcher struct {
	// The path for the file we're watching.
	path string
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
			cur = w.updateState(&state{err: &errNotExist{err}}, cur)
			continue
		}

		// Read the file.
		b, err := ioutil.ReadFile(file)
		if err != nil {
			// File probably does not exist. Try again later.
			cur = w.updateState(&state{err: &errNotExist{err}}, cur)
			continue
		}

		// If it's a new value, decode and return it.
		if cur == nil || cur.err != nil || !bytes.Equal(cur.raw, b) {
			if val, err := decoder.Decode(ctx, b); err != nil {
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

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i interface{}) bool { return false }

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	if _, ok := err.(*errNotExist); ok {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}
