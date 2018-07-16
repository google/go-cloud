// Copyright 2018 Google LLC
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

package fileblob

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/go-cloud/blob"
)

func TestXattrs(t *testing.T) {
	tests := []struct {
		name, contentType string
	}{
		{
			name:        "TypeOnly",
			contentType: "text/plain",
		},
		{
			name:        "TypeAndCharset",
			contentType: "text/plain; charset=utf8",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "fileblob")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)

			b, err := NewBucket(dir)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			w, err := b.NewWriter(ctx, "foo.txt", &blob.WriterOptions{
				ContentType: tc.contentType,
			})
			if err != nil {
				t.Fatalf("b.NewWriter: %v", err)
			}
			if _, err := w.Write([]byte("Hello, World!\n")); err != nil {
				t.Fatalf("w.Write: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("w.Close: %v", err)
			}

			r, err := b.NewRangeReader(ctx, "foo.txt", 0, 0)
			if err != nil {
				t.Fatalf("b.NewRangeReader: %v", err)
			}
			if got := r.ContentType(); got != tc.contentType {
				t.Errorf("r.ContentType() = %q want %q", got, tc.contentType)
			}
		})
	}
}
