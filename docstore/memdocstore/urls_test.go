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
		URL     string
		wantErr bool
	}{
		// OK.
		{"mem://coll/_id", false},
		{"mem://coll/foo.bar", true}, // "coll" already has key "_id"
		{"mem://coll2/foo.bar", false},
		{"mem://", true},                     // missing collection
		{"mem://coll", true},                 // missing key
		{"mem://coll/my/key", true},          // key with slash
		{"mem://coll/key?param=value", true}, // invalid parameter
	}
	ctx := context.Background()
	for _, test := range tests {
		d, err := docstore.OpenCollection(ctx, test.URL)
		if d != nil {
			defer d.Close()
		}
		if (err != nil) != test.wantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.wantErr)
		}
	}
}
