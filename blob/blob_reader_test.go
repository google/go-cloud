// Copyright 2022 The Go Cloud Development Kit Authors
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

package blob_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"testing/iotest"

	"gocloud.dev/blob/memblob"
)

// TestReader verifies that blob.Reader implements io package interfaces correctly.
func TestReader(t *testing.T) {
	const myKey = "testkey"

	bucket := memblob.OpenBucket(nil)
	defer bucket.Close()

	// Get some random data, of a large enough size to require multiple
	// reads/writes given our buffer size of 1024.
	data, err := randomData(1024*10 + 10)
	if err != nil {
		t.Fatal(err)
	}

	// Write the data to a key.
	ctx := context.Background()
	bucket.WriteAll(ctx, myKey, data, nil)

	// Create a blob.Reader.
	r1, err := bucket.NewReader(ctx, myKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	r1.Close()
	if err := iotest.TestReader(r1, data); err != nil {
		t.Error(err)
	}

	// Create another blob.Reader to exercise the ReadFrom code path
	r2, err := bucket.NewReader(ctx, myKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()

	var buffer bytes.Buffer
	n, err := io.Copy(&buffer, r2)
	if err != nil {
		t.Fatal(err)
	} else if n != int64(len(data)) {
		t.Fatal("wrote fewer bytes than expected")
	} else if !bytes.Equal(buffer.Bytes(), data) {
		t.Fatal("wrote invalid bytes")
	}
}
