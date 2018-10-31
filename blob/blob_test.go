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

package blob

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cloud/blob/driver"
	"github.com/google/go-cmp/cmp"
)

// TestOpen tests blob.Open.
func TestOpen(t *testing.T) {
	ctx := context.Background()
	var gotBucket string
	var gotOptions map[string]string

	// Register protocol foo to always return nil.
	// Sets gotBucket and gotOptions as a side effect
	if err := Register("foo", func(_ context.Context, bucket string, opts map[string]string) (driver.Bucket, error) {
		gotBucket = bucket
		gotOptions = opts
		return nil, nil
	}); err != nil {
		t.Fatalf("failed to register foo: %v", err)
	}

	// Register protocol err to always return an error.
	if err := Register("err", func(_ context.Context, _ string, _ map[string]string) (driver.Bucket, error) {
		return nil, errors.New("fail")
	}); err != nil {
		t.Fatalf("failed to register foo: %v", err)
	}

	for _, tc := range []struct {
		name        string
		url         string
		wantErr     bool
		wantBucket  string
		wantOptions map[string]string
	}{
		{
			name:    "empty URL",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     "foo",
			wantErr: true,
		},
		{
			name:    "invalid URL missing bucket",
			url:     "foo://",
			wantErr: true,
		},
		{
			name:    "invalid URL empty bucket",
			url:     "foo://?xxx=yyy&bbb",
			wantErr: true,
		},
		{
			name:    "invalid URL bad option",
			url:     "foo://mybucket?xxx=yyy&bbb",
			wantErr: true,
		},
		{
			name:    "unregistered protocol",
			url:     "bar://mybucket",
			wantErr: true,
		},
		{
			name:    "fn returns error",
			url:     "err://mybucket",
			wantErr: true,
		},
		{
			name:        "no options",
			url:         "foo://mybucket",
			wantBucket:  "mybucket",
			wantOptions: map[string]string{},
		},
		{
			name:        "empty options",
			url:         "foo://mybucket?",
			wantBucket:  "mybucket",
			wantOptions: map[string]string{},
		},
		{
			name:        "options",
			url:         "foo://mybucket?aAa=bBb&cCc=dDd",
			wantBucket:  "mybucket",
			wantOptions: map[string]string{"aaa": "bBb", "ccc": "dDd"},
		},
		{
			name:        "fancy bucket name",
			url:         "foo:///foo/bar/baz",
			wantBucket:  "/foo/bar/baz",
			wantOptions: map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := Open(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if gotBucket != tc.wantBucket {
				t.Errorf("got bucket name %q want %q", gotBucket, tc.wantBucket)
			}
			if diff := cmp.Diff(gotOptions, tc.wantOptions); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", gotOptions, tc.wantOptions, diff)
			}
		})
	}
}
