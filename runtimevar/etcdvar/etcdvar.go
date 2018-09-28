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

// defaultWait is the default value for WatchOptions.WaitTime.
const defaultWait = 30 * time.Second

// New constructs a runtimevar.Variable object that uses client to watch
// variables in etcd.
// Provide a decoder to unmarshal updated configurations into similar
// objects during the Watch call.
func New(name string, cli *clientv3.Client, decoder *runtimevar.Decoder, opts *WatchOptions) (*runtimevar.Variable, error) {
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
	return runtimevar.New(&watcher{
		name:    name,
		client:  cli,
		decoder: decoder,
		wait:    waitTime,
	}), nil
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
	var code, prevCode codes.Code
	if etcdErr, ok := err.(rpctypes.EtcdError); ok {
		code = etcdErr.Code()
	}
	if etcdErr, ok := prev.err.(rpctypes.EtcdError); ok {
		prevCode = etcdErr.Code()
	}
	if code != codes.OK && code == prevCode {
		return nil
	}
	return s
}

// watcher implements driver.Watcher.
type watcher struct {
	name    string
	client  *clientv3.Client
	decoder *runtimevar.Decoder
	wait    time.Duration
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {

	// Create a watching channel in case the variable hasn't changed.
	// We must create it now before the Get to avoid race conditions.
	var ch clientv3.WatchChan
	if prev != nil {
		ch = w.client.Watch(ctx, w.name)
	}

	for {
		resp, err := w.client.Get(ctx, w.name)
		if err != nil {
			return errorState(err, prev), w.wait
		}
		if len(resp.Kvs) == 0 {
			return errorState(fmt.Errorf("%q not found", w.name), prev), w.wait
		} else if len(resp.Kvs) > 1 {
			return errorState(fmt.Errorf("%q has multiple values", w.name), prev), w.wait
		}
		kv := resp.Kvs[0]
		if prev == nil || kv.Version != prev.(*state).version {
			// New Value
			val, err := w.decoder.Decode(kv.Value)
			if err != nil {
				return errorState(err, prev), w.wait
			}
			return &state{val: val, updateTime: time.Now(), version: kv.Version}, 0
		}

		// Value hasn't changed. Wait for change events.
		<-ch
	}
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// WatchOptions allows the specification of various options to a watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of retries after an error. Defaults to 30 seconds.
	WaitTime time.Duration
}
