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

package etcdvar_test

import (
	"context"
	"log"

	"go.etcd.io/etcd/clientv3"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/etcdvar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func Example() {
	// Connect to the etcd server.
	client, err := clientv3.NewFromURL("http://your.etcd.server:9999")
	if err != nil {
		log.Fatal(err)
	}

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable that watches the variable.
	// The etcd variable being referenced should have a JSON string that
	// decodes into MyConfig.
	v, err := etcdvar.New(client, "cfg-variable-name", decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cfg := snapshot.Value.(MyConfig)
	_ = cfg
}

func Example_openVariable() {
	// OpenVariable creates a *runtimevar.Variable from a URL.
	// This example watches a variable based on a file-based blob.Bucket with JSON.
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "etcd://myvarname?client=my.etcd.server:9999")
	if err != nil {
		log.Fatal(err)
	}

	snapshot, err := v.Watch(ctx)
	if err != nil {
		log.Fatal(err)
	}
	_, _ = snapshot, err
}
