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
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
)

// TestReader verifies that blob.Reader implements io package interfaces correctly.
func TestReader(t *testing.T) {
	const myKey = "testkey"

	bucket := memblob.OpenBucket(nil)
	defer func() {
		if err := bucket.Close(); err != nil {
			t.Errorf("failed to Close: %v", err)
		}
	}()

	// Get some random data, of a large enough size to require multiple
	// reads/writes given our buffer size of 1024.
	data, err := randomData(1024*10 + 10)
	if err != nil {
		t.Fatal(err)
	}

	// Write the data to a key.
	ctx := context.Background()
	if err := bucket.WriteAll(ctx, myKey, data, nil); err != nil {
		t.Errorf("failed to write: %v", err)
	}

	// Create a blob.Reader.
	r1, err := bucket.NewReader(ctx, myKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r1.Close(); err != nil {
		t.Errorf("failed to Close: %v", err)
	}
	if err := iotest.TestReader(r1, data); err != nil {
		t.Error(err)
	}

	// Create another blob.Reader to exercise the ReadFrom code path
	r2, err := bucket.NewReader(ctx, myKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r2.Close(); err != nil {
			t.Errorf("failed to Close: %v", err)
		}
	}()

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

// TestSetDefaultReaderOptionsConcurrent verifies that SetDefaultReaderOptions
// can be called concurrently with ReadAll and Download without deadlocking.
func TestSetDefaultReaderOptionsConcurrent(t *testing.T) {
	ctx := context.Background()
	bucket := memblob.OpenBucket(nil)
	if err := bucket.WriteAll(ctx, "key", []byte("hello"), nil); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if _, err := bucket.ReadAll(ctx, "key"); err != nil {
					t.Error(err)
					return
				}
				if err := bucket.Download(ctx, "key", io.Discard, nil); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 1000; j++ {
			if err := bucket.SetDefaultReaderOptions(blob.ReaderOptions{BufferSize: j}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		// Don't Close the bucket; it would block on the deadlocked calls.
		t.Fatal("deadlock: ReadAll/Download did not finish while SetDefaultReaderOptions was called concurrently")
	}
	closeWithErrorCheck(t, bucket)
}
