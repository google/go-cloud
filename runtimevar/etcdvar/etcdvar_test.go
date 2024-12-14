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

package etcdvar

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
)

// To run these tests against a local etcd server, first run ./localetcd.sh.
// Then wait a few seconds for the server to be ready.

type harness struct {
	client *clientv3.Client
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	if !setup.HasDockerTestEnvironment() {
		t.Skip("Skipping etcd tests since the etcd server is not available")
	}
	c, err := clientv3.NewFromURL("http://localhost:2379")
	if err != nil {
		t.Fatalf("No local etcd server running: %v; see runtimevar/etcdvar/localetcd.sh", err)
	}
	return &harness{client: c}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, name, decoder, nil), nil
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

func (h *harness) Close() {}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var resp *clientv3.GetResponse
	if !s.As(&resp) {
		return errors.New("Snapshot.As failed")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	// etcdvar returns a fmt.Errorf error for "not found", so this is expected
	// to fail.
	var to rpctypes.EtcdError
	if v.ErrorAs(err, &to) {
		return errors.New("ErrorAs expected to fail")
	}
	// Try with a real etcd error.
	if !v.ErrorAs(rpctypes.ErrUnhealthy, &to) {
		return errors.New("ErrorAs expected to succeed with real etcd error")
	}
	return nil
}

// Etcd-specific tests.

func TestEquivalentError(t *testing.T) {
	tests := []struct {
		Err1, Err2 error
		Want       bool
	}{
		{Err1: errors.New("not etcd"), Err2: errors.New("not etcd"), Want: true},
		{Err1: errors.New("not etcd"), Err2: errors.New("not etcd but different")},
		{Err1: errors.New("not etcd"), Err2: rpctypes.ErrUnhealthy},
		{Err1: rpctypes.ErrUnhealthy, Err2: rpctypes.ErrRequestTooLarge},
		{Err1: rpctypes.ErrUnhealthy, Err2: rpctypes.ErrUnhealthy, Want: true},
	}

	for _, test := range tests {
		got := equivalentError(test.Err1, test.Err2)
		if got != test.Want {
			t.Errorf("%v vs %v: got %v want %v", test.Err1, test.Err2, got, test.Want)
		}
	}
}

func TestNoConnectionError(t *testing.T) {
	cli, err := clientv3.NewFromURL("http://no.server.here:999")
	if err != nil {
		t.Fatal(err)
	}
	v, err := OpenVariable(cli, "variable-name", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	// Watch will block for quite a while trying to connect,
	// so use a short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	_, err = v.Watch(ctx)
	if err == nil {
		t.Error("got nil want error")
	}
}

func TestOpenVariable(t *testing.T) {
	h, err := newHarness(t)
	if err != nil {
		t.Fatal(err)
	}
	os.Setenv("ETCD_SERVER_URL", "http://localhost:2379")

	ctx := context.Background()
	if err := h.CreateVariable(ctx, "string-var", []byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := h.CreateVariable(ctx, "json-var", []byte(`{"Foo": "Bar"}`)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		URL          string
		WantErr      bool
		WantWatchErr bool
		Want         any
	}{
		// Nonexistentvar does not exist, so we get an error from Watch.
		{"etcd://nonexistentvar", false, true, nil},
		// Invalid decoder arg.
		{"etcd://string-var?decoder=notadecoder", true, false, nil},
		// Invalid parameter.
		{"etcd://string-var?param=value", true, false, nil},
		// Working example with string decoder.
		{"etcd://string-var?decoder=string", false, false, "hello world"},
		// Working example with default decoder.
		{"etcd://string-var", false, false, []byte("hello world")},
		// Working example with JSON decoder.
		{"etcd://json-var?decoder=jsonmap", false, false, &map[string]any{"Foo": "Bar"}},
	}

	for _, test := range tests {
		t.Run(test.URL, func(t *testing.T) {
			v, err := runtimevar.OpenVariable(ctx, test.URL)
			if (err != nil) != test.WantErr {
				t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
			}
			if err != nil {
				return
			}
			defer v.Close()
			snapshot, err := v.Watch(ctx)
			if (err != nil) != test.WantWatchErr {
				t.Errorf("%s: got Watch error %v, want error %v", test.URL, err, test.WantWatchErr)
			}
			if err != nil {
				return
			}
			if !cmp.Equal(snapshot.Value, test.Want) {
				t.Errorf("%s: got snapshot value\n%v\n  want\n%v", test.URL, snapshot.Value, test.Want)
			}
		})
	}
}
