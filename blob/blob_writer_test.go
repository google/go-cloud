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

package blob_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/memblob"
)

// TestWriteReturnValues verifies that blob.Writer returns the correct n
// even when it is doing content sniffing.
func TestWriteReturnValues(t *testing.T) {
	ctx := context.Background()

	for _, withContentType := range []bool{true, false} {
		t.Run(fmt.Sprintf("withContentType %v", withContentType), func(t *testing.T) {
			bucket := memblob.OpenBucket(nil)
			defer bucket.Close()

			var opts *blob.WriterOptions
			if withContentType {
				opts = &blob.WriterOptions{ContentType: "application/octet-stream"}
			}
			w, err := bucket.NewWriter(ctx, "testkey", opts)
			if err != nil {
				t.Fatalf("couldn't create writer with options: %v", err)
			}
			defer func() {
				if err := w.Close(); err != nil {
					t.Errorf("failed to close writer: %v", err)
				}
			}()
			n, err := io.CopyN(w, rand.Reader, 182)
			if err != nil || n != 182 {
				t.Fatalf("CopyN(182) got %d, want 182: %v", n, err)
			}
			n, err = io.CopyN(w, rand.Reader, 1812)
			if err != nil || n != 1812 {
				t.Fatalf("CopyN(1812) got %d, want 1812: %v", n, err)
			}
		})
	}
}
