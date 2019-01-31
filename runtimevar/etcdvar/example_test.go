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
	"fmt"
	"log"
	"time"

	"go.etcd.io/etcd/clientv3"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/etcdvar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleNew() {
	// Connect to the etcd server.
	client, err := clientv3.NewFromURL("http://foo.bar.com:9999")
	if err != nil {
		log.Fatal(err)
	}

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable that watches the variable.
	// For this example, the etcd variable being referenced should have a
	// JSON string that decodes into MyConfig.
	// The example uses a very short timeout since the there's no real etcd server
	// running at the URL provided, so we might as well give up quickly.
	v, err := etcdvar.New(client, "myconfig", decoder, &etcdvar.Options{Timeout: 1 * time.Millisecond})
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// You can now read the current value of the variable from v.
	snapshot, err := v.Watch(context.Background())
	if err != nil {
		// This is expected because we can't connect to the fake server URL.
		fmt.Println("Watch returned an error")
		return
	}
	// We'll never get here when running this sample, but the resulting
	// runtimevar.Snapshot.Value would be of type MyConfig.
	log.Printf("Snapshot.Value: %#v", snapshot.Value.(MyConfig))

	// Output:
	// Watch returned an error
}
