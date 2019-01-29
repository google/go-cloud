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

// Package blobvar provides a runtimevar implementation with
// variables read from a blob.Bucket.
// Use NewVariable to construct a *runtimevar.Variable.
//
// As
//
// blobvar exposes the following types for As:
//  - Snapshot: Not supported.
//  - Error: error, which can be passed to blob.ErrorAs.
package blobvar // import "gocloud.dev/runtimevar/blobvar"

import (
	"bytes"
	"context"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which the blob is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// NewVariable constructs a *runtimevar.Variable backed by the referenced blob.
// Reads of the blob return raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func NewVariable(bucket *blob.Bucket, key string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(bucket, key, decoder, opts)), nil
}

func newWatcher(bucket *blob.Bucket, key string, decoder *runtimevar.Decoder, opts *Options) driver.Watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		bucket:  bucket,
		key:     key,
		wait:    driver.WaitDuration(opts.WaitDuration),
		decoder: decoder,
	}
}

// state implements driver.State.
type state struct {
	val        interface{}
	updateTime time.Time
	rawBytes   []byte
	err        error
}

// Value implements driver.State.Value.
func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

// UpdateTime implements driver.State.UpdateTime.
func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// As implements driver.State.As.
func (s *state) As(i interface{}) bool {
	return false
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
	if err == prev.err || err.Error() == prev.err.Error() {
		// Same error, return nil to indicate no change.
		return nil
	}
	return s
}

// watcher implements driver.Watcher for configurations provided by the Runtime Configurator
// service.
type watcher struct {
	bucket  *blob.Bucket
	key     string
	wait    time.Duration
	decoder *runtimevar.Decoder
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	// Read the blob.
	b, err := w.bucket.ReadAll(ctx, w.key)
	if err != nil {
		return errorState(err, prev), w.wait
	}
	// See if it's the same raw bytes as before.
	if prev != nil && bytes.Equal(b, prev.(*state).rawBytes) {
		// No change!
		return nil, w.wait
	}

	// Decode the value.
	val, err := w.decoder.Decode(b)
	if err != nil {
		return errorState(err, prev), w.wait
	}
	return &state{val: val, updateTime: time.Now(), rawBytes: b}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
// Since blobvar uses the blob package, ErrorAs delegates
// to the bucket's ErrorAs method.
func (w *watcher) ErrorAs(err error, i interface{}) bool {
	return w.bucket.ErrorAs(err, i)
}

// IsNotExist implements driver.IsNotExist.
func (*watcher) IsNotExist(err error) bool {
	return gcerrors.Code(err) == gcerrors.NotFound
}
