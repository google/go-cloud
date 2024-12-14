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

// Package etcdvar provides a runtimevar implementation with variables
// backed by etcd. Use OpenVariable to construct a *runtimevar.Variable.
//
// # URLs
//
// For runtimevar.OpenVariable, etcdvar registers for the scheme "etcd".
// The default URL opener will dial an etcd server based on the environment
// variable "ETCD_SERVER_URL".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// etcdvar exposes the following types for As:
//   - Snapshot: *clientv3.GetResponse
//   - Error: rpctypes.EtcdError
package etcdvar // import "gocloud.dev/runtimevar/etcdvar"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
	"gocloud.dev/gcerrors"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"google.golang.org/grpc/codes"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, &defaultDialer{})
}

// Scheme is the URL scheme etcdvar registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "etcd"

type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultDialer) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	o.init.Do(func() {
		serverURL := os.Getenv("ETCD_SERVER_URL")
		if serverURL == "" {
			o.err = errors.New("ETCD_SERVER_URL environment variable is not set")
			return
		}
		client, err := clientv3.NewFromURL(serverURL)
		if err != nil {
			o.err = fmt.Errorf("failed to connect to default client %q: %v", serverURL, err)
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, o.err)
	}
	return o.opener.OpenVariableURL(ctx, u)
}

// URLOpener opens etcd URLs like "etcd://mykey?decoder=string".
//
// The host+path is used as the variable name.
//
// The following URL parameters are supported:
//   - decoder: The decoder to use. Defaults to runtimevar.BytesDecoder.
//     See runtimevar.DecoderByName for supported values.
type URLOpener struct {
	// The Client to use; required.
	Client *clientv3.Client

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the options to pass to OpenVariable.
	Options Options
}

// OpenVariableURL opens a etcdvar Variable for u.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(ctx, decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}
	return OpenVariable(o.Client, path.Join(u.Host, u.Path), decoder, &o.Options)
}

// Options sets options.
type Options struct {
	// Timeout controls the timeout on RPCs to etcd; timeouts will result in
	// errors being returned from Watch. Defaults to 30 seconds.
	Timeout time.Duration
}

// OpenVariable constructs a *runtimevar.Variable that uses client to watch the variable
// name on an etcd server.
// etcd returns raw bytes; provide a decoder to decode the raw bytes into the
// appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(cli *clientv3.Client, name string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(cli, name, decoder, opts)), nil
}

func newWatcher(cli *clientv3.Client, name string, decoder *runtimevar.Decoder, opts *Options) *watcher {
	if opts == nil {
		opts = &Options{}
	}
	// Create a ctx for the background goroutine that does all of the reading.
	// The cancel function will be used to shut it down during Close.
	ctx, cancel := context.WithCancel(context.Background())
	w := &watcher{
		// See struct comments for why it's buffered.
		ch:       make(chan *state, 1),
		shutdown: cancel,
	}
	go w.watch(ctx, cli, name, decoder, driver.WaitDuration(opts.Timeout))
	return w
}

// errNotExist is a sentinel error for nonexistent variables.
var errNotExist = errors.New("variable does not exist")

// state implements driver.State.
type state struct {
	val        any
	raw        *clientv3.GetResponse
	updateTime time.Time
	version    int64
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
	if s.raw == nil {
		return false
	}
	p, ok := i.(**clientv3.GetResponse)
	if !ok {
		return false
	}
	*p = s.raw
	return true
}

// watcher implements driver.Watcher.
type watcher struct {
	// The background goroutine writes new *state values to ch.
	// It is buffered so that the background goroutine can write without
	// blocking; it always drains the buffer before writing so that the latest
	// write is buffered. If writes could block, the background goroutine could be
	// blocked indefinitely from reading etcd's Watch events.
	// The background goroutine closes ch during shutdown.
	ch chan *state
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
	if s.err != nil && prev != nil && prev.err != nil {
		if equivalentError(s.err, prev.err) {
			// s represents the same error as prev.
			return s
		}
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

// equivalentError returns true iff err1 and err2 represent an equivalent error;
// i.e., we don't want to return it to the user as a different error.
func equivalentError(err1, err2 error) bool {
	if err1 == err2 || err1.Error() == err2.Error() {
		return true
	}
	var code1, code2 codes.Code
	if etcdErr, ok := err1.(rpctypes.EtcdError); ok {
		code1 = etcdErr.Code()
	}
	if etcdErr, ok := err2.(rpctypes.EtcdError); ok {
		code2 = etcdErr.Code()
	}
	return code1 != codes.OK && code1 == code2
}

// watch is run by a background goroutine.
// It watches file using cli.Watch, and writes new states to w.ch.
// It exits when ctx is canceled, and closes w.ch.
func (w *watcher) watch(ctx context.Context, cli *clientv3.Client, name string, decoder *runtimevar.Decoder, timeout time.Duration) {
	var cur *state
	defer close(w.ch)

	var watchCh clientv3.WatchChan
	for {
		if watchCh == nil {
			ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
			watchCh = cli.Watch(ctxWithTimeout, name)
			cancel()
		}

		ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
		resp, err := cli.Get(ctxWithTimeout, name)
		cancel()
		if err != nil {
			cur = w.updateState(&state{err: err}, cur)
		} else if len(resp.Kvs) == 0 {
			cur = w.updateState(&state{err: errNotExist}, cur)
		} else if len(resp.Kvs) > 1 {
			cur = w.updateState(&state{err: fmt.Errorf("%q has multiple values", name)}, cur)
		} else {
			kv := resp.Kvs[0]
			if cur == nil || cur.err != nil || kv.Version != cur.version {
				val, err := decoder.Decode(ctx, kv.Value)
				if err != nil {
					cur = w.updateState(&state{err: err}, cur)
				} else {
					cur = w.updateState(&state{val: val, raw: resp, updateTime: time.Now(), version: kv.Version}, cur)
				}
			}
		}

		// Value hasn't changed. Wait for change events.
		select {
		case <-ctx.Done():
			return
		case _, ok := <-watchCh:
			if !ok {
				// watchCh has closed; retry in next loop iteration.
				watchCh = nil
			}
		}
	}
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	// Tell the background goroutine to shut down by canceling its ctx.
	w.shutdown()
	// Wait for it to exit.
	for range w.ch {
	}
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i any) bool {
	switch v := err.(type) {
	case rpctypes.EtcdError:
		if p, ok := i.(*rpctypes.EtcdError); ok {
			*p = v
			return true
		}
	}
	return false
}

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	if err == errNotExist {
		return gcerrors.NotFound
	}
	return gcerrors.Unknown
}
