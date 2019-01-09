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

// jsonCreds is a fake GCP JSON credentials file.
const jsonCreds = `
{
  "type": "service_account",
  "project_id": "my-project-id"
}
`

func ExampleNewVariable() {
	ctx := context.Background()

	// Create a *blob.Bucket.
	// Here, we use an in-memory implementation.
	bucket := memblob.OpenBucket(nil)

	// Create a decoder for decoding JSON strings into MyConfig.
	decoder := runtimevar.NewDecoder(MyConfig{}, runtimevar.JSONDecode)

	// Construct a *runtimevar.Variable that watches the blob.
	v, err := blobvar.NewVariable(bucket, "blob name", decoder, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer v.Close()

	// You can now read the current value of the variable from v.
	snapshot, err := v.Watch(ctx)
	if err != nil {
		// This is expected due to the fake credentials we used above.
		fmt.Println("Watch failed due to blob not existing")
		return
	}
	// We'll never get here when running this sample, but the resulting
	// runtimevar.Snapshot.Value would be of type MyConfig.
	log.Printf("Snapshot.Value: %#v", snapshot.Value.(MyConfig))

	// Output:
	// Watch failed due to blob not existing
}
