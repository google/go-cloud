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

// Package etcdvar provides a runtimevar driver implementation to read
// variables from etcd.
package etcdvar

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"go.etcd.io/etcd/clientv3"
)

// New constructs a runtimevar.Variable object that uses client to watch
// variables in etcd.
// Provide a decoder to unmarshal updated configurations into similar
// objects during the Watch call.
func New(name string, cli *clientv3.Client, decoder *runtimevar.Decoder) (*runtimevar.Variable, error) {
	return runtimevar.New(&impl{
		name:    name,
		client:  cli,
		decoder: decoder,
	}), nil
}

// impl implements driver.Watcher.
type impl struct {
	name    string
	client  *clientv3.Client
	decoder *runtimevar.Decoder
}

func (i *impl) WatchVariable(ctx context.Context, prevVersion interface{}, prevErr error) (*driver.Variable, interface{}, time.Duration, error) {

	// checkSameErr checks to see if err is the same as prevErr, andif so, returns
	// the "no change" signal with w.waitTime.
	checkSameErr := func(err error) (*driver.Variable, interface{}, time.Duration, error) {
		if prevErr != nil {
			if err.Error() == prevErr.Error() {
				return nil, nil, 10 * time.Second, nil
			}
		}
		return nil, nil, 0, err
	}

	// Create a watching channel in case the variable hasn't changed.
	// We must create it now before the Get to avoid race conditions.
	ch := i.client.Watch(ctx, i.name)

	for {
		resp, err := i.client.Get(ctx, i.name)
		if err != nil {
			return checkSameErr(err)
		}
		if len(resp.Kvs) == 0 {
			return checkSameErr(fmt.Errorf("%q not found", i.name))
		} else if len(resp.Kvs) > 1 {
			return checkSameErr(fmt.Errorf("%q has multiple values", i.name))
		}
		kv := resp.Kvs[0]
		if prevVersion == nil || kv.Version != prevVersion.(int64) {
			// New Value
			val, err := i.decoder.Decode(kv.Value)
			if err != nil {
				return checkSameErr(err)
			}
			return &driver.Variable{Value: val, UpdateTime: time.Now()}, kv.Version, 0, nil
		}

		// Value hasn't changed. Wait for change events.
		_ = <-ch
	}
}

func (i *impl) Close() error {
	return nil
}
