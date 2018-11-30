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

// Package etcdvar provides a runtimevar.Driver implementation to read
// variables from etcd.
package etcdvar

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"google.golang.org/grpc/codes"
)

// Options sets options.
// It is provided for future extensibility.
type Options struct{}

// New constructs a runtimevar.Variable object that uses client to watch
// variables in etcd.
// Provide a decoder to unmarshal updated configurations into similar
// objects during the Watch call.
func New(name string, cli *clientv3.Client, decoder *runtimevar.Decoder, _ *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(name, cli, decoder)), nil
}

func newWatcher(name string, cli *clientv3.Client, decoder *runtimevar.Decoder) *watcher {
	// Create a ctx for the background goroutine that does all of the reading.
	// The cancel function will be used to shut it down during Close.
	ctx, cancel := context.WithCancel(context.Background())
	w := &watcher{
		// See struct comments for why it's buffered.
		ch:       make(chan *state, 1),
		shutdown: cancel,
	}
	go w.watch(ctx, cli, name, decoder)
	return w
}

// state implements driver.State.
type state struct {
	val        interface{}
	updateTime time.Time
	version    int64
	err        error
}

func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

func (s *state) UpdateTime() time.Time {
	return s.updateTime
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
		if s.err == prev.err || s.err.Error() == prev.err.Error() {
			// s represents the same error as prev.
			return s
		}
		var code, prevCode codes.Code
		if etcdErr, ok := s.err.(rpctypes.EtcdError); ok {
			code = etcdErr.Code()
		}
		if etcdErr, ok := prev.err.(rpctypes.EtcdError); ok {
			prevCode = etcdErr.Code()
		}
		if code != codes.OK && code == prevCode {
			// s represents the same etcd error code error as prev.
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

// watch is run by a background goroutine.
// It watches file using cli.Watch, and writes new states to w.ch.
// It exits when ctx is canceled, and closes w.ch.
func (w *watcher) watch(ctx context.Context, cli *clientv3.Client, name string, decoder *runtimevar.Decoder) {
	var cur *state
	defer close(w.ch)

	watchCh := cli.Watch(ctx, name)
	for {
		resp, err := cli.Get(ctx, name)
		if err != nil {
			cur = w.updateState(&state{err: err}, cur)
		} else if len(resp.Kvs) == 0 {
			cur = w.updateState(&state{err: fmt.Errorf("%q not found", name)}, cur)
		} else if len(resp.Kvs) > 1 {
			cur = w.updateState(&state{err: fmt.Errorf("%q has multiple values", name)}, cur)
		} else {
			kv := resp.Kvs[0]
			if cur == nil || cur.err != nil || kv.Version != cur.version {
				val, err := decoder.Decode(kv.Value)
				if err != nil {
					cur = w.updateState(&state{err: err}, cur)
				} else {
					cur = w.updateState(&state{val: val, updateTime: time.Now(), version: kv.Version}, cur)
				}
			}
		}

		// Value hasn't changed. Wait for change events.
		select {
		case <-ctx.Done():
			return
		case <-watchCh:
		}
	}
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	// Tell the background goroutine to shut down by canceling its ctx.
	w.shutdown()
	// Wait for it to exit.
	for _ = range w.ch {
	}
	return nil
}
