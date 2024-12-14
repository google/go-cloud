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
// Use OpenVariable to construct a *runtimevar.Variable.
//
// # URLs
//
// For runtimevar.OpenVariable, blobvar registers for the scheme "blob".
// The default URL opener will open a blob.Bucket based on the environment
// variable "BLOBVAR_BUCKET_URL".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// blobvar exposes the following types for As:
//   - Snapshot: Not supported.
//   - Error: error, which can be passed to blob.ErrorAs.
package blobvar // import "gocloud.dev/runtimevar/blobvar"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, &defaultOpener{})
}

type defaultOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	o.init.Do(func() {
		bucketURL := os.Getenv("BLOBVAR_BUCKET_URL")
		if bucketURL == "" {
			o.err = errors.New("BLOBVAR_BUCKET_URL environment variable is not set")
			return
		}
		bucket, err := blob.OpenBucket(ctx, bucketURL)
		if err != nil {
			o.err = fmt.Errorf("failed to open default bucket %q: %v", bucketURL, err)
			return
		}
		o.opener = &URLOpener{Bucket: bucket}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, o.err)
	}
	return o.opener.OpenVariableURL(ctx, u)
}

// Scheme is the URL scheme blobvar registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "blob"

// URLOpener opens blob-backed URLs like "blob://myblobkey?decoder=string".
// It supports the following URL parameters:
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//     runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//     See runtimevar.DecoderByName for supported values.
//   - wait: The poll interval, in time.ParseDuration formats.
//     Defaults to 30s.
type URLOpener struct {
	// Bucket is required.
	Bucket *blob.Bucket

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the Options for OpenVariable.
	Options Options
}

// OpenVariableURL opens the variable at the URL's path. See the package doc
// for more details.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	if o.Bucket == nil {
		return nil, fmt.Errorf("open variable %v: bucket is required", u)
	}

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
	return OpenVariable(o.Bucket, path.Join(u.Host, u.Path), decoder, &opts)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which the blob is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the referenced blob.
// Reads of the blob return raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(bucket *blob.Bucket, key string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
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
	val        any
	updateTime time.Time
	rawBytes   []byte
	err        error
}

// Value implements driver.State.Value.
func (s *state) Value() (any, error) {
	return s.val, s.err
}

// UpdateTime implements driver.State.UpdateTime.
func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// As implements driver.State.As.
func (s *state) As(i any) bool {
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
	val, err := w.decoder.Decode(ctx, b)
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
func (w *watcher) ErrorAs(err error, i any) bool {
	return w.bucket.ErrorAs(err, i)
}

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	// err might have come from blob, in which case use its code.
	return gcerrors.Code(err)
}
