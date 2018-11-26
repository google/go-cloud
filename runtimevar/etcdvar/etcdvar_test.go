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

package etcdvar

import (
	"context"
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/embed"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cloud/runtimevar/drivertest"
)

var (
	etcd    *embed.Etcd
	etcdErr error
)

func init() {
	cfg := embed.NewConfig()
	cfg.Dir = "default.etcd"
	etcd, etcdErr = embed.StartEtcd(cfg)
	if etcdErr != nil {
		return
	}
	select {
	case <-etcd.Server.ReadyNotify():
	}
}

type harness struct {
	client *clientv3.Client
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	if etcdErr != nil {
		return nil, etcdErr
	}
	cli, err := clientv3.NewFromURL("http://localhost:2379")
	if err != nil {
		return nil, err
	}
	return &harness{client: cli}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(name, h.client, decoder), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	_, err := h.client.Put(ctx, name, string(val))
	return err
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	return h.CreateVariable(ctx, name, val)
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	_, err := h.client.Delete(ctx, name)
	return err
}

func (h *harness) Mutable() bool { return true }

func (h *harness) Close() {
	h.client.Close()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}
