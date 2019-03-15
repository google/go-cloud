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
// URLs
//
// For runtimevar.OpenVariable, blobvar registers for the scheme "blob".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
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
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, &URLOpener{Mux: blob.DefaultURLMux()})
}

// Scheme is the URL scheme blobvar registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "blob"

// URLOpener opens blob-backed URLs like "blob://myblobkey?decoder=string".
// It supports the following URL parameters:
//   - bucket: The URL to be passed to blob.OpenBucket. Required unless
//       URLOpener.Bucket is provided explicitly.
//       blob.OpenBucket will be called once per unique bucket URL.
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//       runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//       See runtimevar.DecoderByName for supported values.
type URLOpener struct {
	// Mux is required unless Bucket is provided. The "bucket" URL parameter
	// must be provided, and Mux will be used to open it. Opened buckets are
	// cached.
	Mux *blob.URLMux

	// Bucket is optional; if it is provided, it is always used.
	Bucket *blob.Bucket

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the Options for NewVariable.
	Options Options

	mu      sync.Mutex
	buckets map[string]*refCountBucket
}

type refCountBucket struct {
	bucket *blob.Bucket
	refs   int
}

// OpenVariableURL opens the variable at the URL's path. See the package doc
// for more details.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	bucket := o.Bucket
	var watcherOpener *URLOpener // Set if bucket opened via URL to decrement references.

	if bucket == nil {
		if o.Mux == nil {
			return nil, fmt.Errorf("open variable %v: URLOpener.Mux is required if Bucket is not provided", u)
		}
		bucketURL := u.Query().Get("bucket")
		if bucketURL == "" {
			return nil, fmt.Errorf("open variable %v: URL parameter \"bucket\" is required", u)
		}
		var err error
		bucket, err = o.bucketForURL(ctx, bucketURL)
		if err != nil {
			return nil, fmt.Errorf("open variable %v: %v", u, err)
		}
		watcherOpener = o
		q.Del("bucket")
	}

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}
	return runtimevar.New(newWatcher(bucket, path.Join(u.Host, u.Path), decoder, watcherOpener, &o.Options)), nil
}

func (o *URLOpener) bucketForURL(ctx context.Context, bucketURL string) (*blob.Bucket, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.buckets == nil {
		o.buckets = make(map[string]*refCountBucket)
	}
	rcBucket := o.buckets[bucketURL]
	if rcBucket == nil {
		var err error
		newBucket, err := o.Mux.OpenBucket(ctx, bucketURL)
		if err != nil {
			return nil, err
		}
		rcBucket = &refCountBucket{bucket: newBucket}
		o.buckets[bucketURL] = rcBucket
	}
	rcBucket.refs++
	return rcBucket.bucket, nil
}

func (o *URLOpener) decBucketRef(bucket *blob.Bucket) error {
	occurrences, drops := 0, 0
	o.mu.Lock()
	for key, rcBucket := range o.buckets {
		if rcBucket.bucket != bucket {
			continue
		}
		occurrences++
		rcBucket.refs--
		if rcBucket.refs <= 0 {
			delete(o.buckets, key)
			drops++
		}
	}
	o.mu.Unlock()
	if drops > 0 && occurrences == drops {
		return bucket.Close()
	}
	return nil
}

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
	return runtimevar.New(newWatcher(bucket, key, decoder, nil, opts)), nil
}

func newWatcher(bucket *blob.Bucket, key string, decoder *runtimevar.Decoder, opener *URLOpener, opts *Options) driver.Watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		bucket:  bucket,
		opener:  opener,
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
	opener  *URLOpener
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
	var err error
	if w.opener != nil {
		err = w.opener.decBucketRef(w.bucket)
		w.opener = nil // Ensure that we don't call multiple times.
	}
	return err
}

// ErrorAs implements driver.ErrorAs.
// Since blobvar uses the blob package, ErrorAs delegates
// to the bucket's ErrorAs method.
func (w *watcher) ErrorAs(err error, i interface{}) bool {
	return w.bucket.ErrorAs(err, i)
}

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	// err might have come from blob, in which case use its code.
	return gcerrors.Code(err)
}
