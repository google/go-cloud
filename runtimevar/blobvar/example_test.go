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

package blobvar_test

import (
	"context"
	"fmt"
	"log"

	"gocloud.dev/blob/memblob"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/blobvar"
)

// MyConfig is a sample configuration struct.
type MyConfig struct {
	Server string
	Port   int
}

func ExampleOpenVariable() {
	// Create a *blob.Bucket.
	// Here, we use an in-memory implementation and write a sample
	// configuration value.
	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()
	ctx := context.Background()
	err := bucket.WriteAll(ctx, "cfg-variable-name", []byte(`{"Server": "foo.com", "Port": 80}`), nil)
	if err != nil {
		log.Fatal(err)
	}

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable that watches the blob.
	v, err := blobvar.OpenVariable(bucket, "cfg-variable-name", decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// runtimevar.Snapshot.Value is decoded to type MyConfig.
	cfg := snapshot.Value.(MyConfig)
	fmt.Printf("%s running on port %d", cfg.Server, cfg.Port)

	// Output:
	// foo.com running on port 80
}

func Example_openVariableFromURL() {
	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	// The default opener opens a blob.Bucket via a URL, based on the environment
	// variable BLOBVAR_BUCKET_URL.
	// This example watches a JSON variable.
	ctx := context.Background()
	v, err := runtimevar.OpenVariable(ctx, "blob://myvar.json?decoder=json")
	if err != nil {
		log.Fatal(err)
	}

	snapshot, err := v.Latest(ctx)
	_, _ = snapshot, err
}
