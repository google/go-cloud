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

package memdocstore

import (
	"context"
	"testing"

	"gocloud.dev/docstore"
)

func TestOpenCollectionFromURL(t *testing.T) {
	tests := []struct {
		URL      string
		collName string
		wantErr  bool
	}{
		// OK.
		{"mem://coll/_id", "coll", false},
		// "coll" already has key "_id".
		{"mem://coll/foo.bar", "coll", true},
		{"mem://coll2/foo.bar", "coll2", false},
		// Missing collection.
		{"mem://", "", true},
		// Missing key.
		{"mem://coll", "coll", true},
		// Key with slash.
		{"mem://coll/my/key", "coll", true},
		// Passing revision field.
		{"mem://coll/_id?revision_field=123", "coll", false},
		// Passing filename.
		{"mem://coll/_id?filename=foo.out", "coll", false},
		// Invalid parameter.
		{"mem://coll/key?param=value", "coll", true},
	}
	ctx := context.Background()
	colls := map[string]*docstore.Collection{}
	for _, test := range tests {
		d, err := docstore.OpenCollection(ctx, test.URL)
		if d != nil {
			colls[test.collName] = d
		}
		if (err != nil) != test.wantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.wantErr)
		}
	}
	for collName, coll := range colls {
		if err := coll.Close(); err != nil {
			t.Errorf("failed to Close %q: %v", collName, err)
		}
	}
}
