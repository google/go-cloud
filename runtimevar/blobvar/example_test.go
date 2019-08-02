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

func ExampleOpenVariable() {
	// Create a *blob.Bucket.
	// Here, we use an in-memory implementation and write a sample value.
	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()
	ctx := context.Background()
	err := bucket.WriteAll(ctx, "cfg-variable-name", []byte("hello world"), nil)
	if err != nil {
		log.Fatal(err)
	}

	// Construct a *runtimevar.Variable that watches the blob.
	v, err := blobvar.OpenVariable(bucket, "cfg-variable-name", runtimevar.StringDecoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// We can now read the current value of the variable from v.
	snapshot, err := v.Latest(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// runtimevar.Snapshot.Value is decoded to a string.
	fmt.Println(snapshot.Value.(string))

	// Output:
	// hello world
}

func Example_openVariableFromURL() {
	// PRAGMA: This example is used on gocloud.dev; PRAGMA comments adjust how it is shown and can be ignored.
	// PRAGMA: On gocloud.dev, add a blank import: _ "gocloud.dev/runtimevar/blobvar"
	// PRAGMA: On gocloud.dev, hide lines until the next blank line.
	ctx := context.Background()

	// runtimevar.OpenVariable creates a *runtimevar.Variable from a URL.
	// The default opener opens a blob.Bucket via a URL, based on the environment
	// variable BLOBVAR_BUCKET_URL.

	v, err := runtimevar.OpenVariable(ctx, "blob://myvar.txt?decoder=string")
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()
}
