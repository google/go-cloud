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

package mongodocstore

import (
	"context"
	"os"
	"testing"

	"gocloud.dev/docstore"
)

func fakeConnectionStringInEnv() func() {
	oldURLVal := os.Getenv("MONGO_SERVER_URL")
	os.Setenv("MONGO_SERVER_URL", "mongodb://localhost")
	return func() {
		os.Setenv("MONGO_SERVER_URL", oldURLVal)
	}
}

func TestOpenCollectionURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mongo://mydb/mycollection", false},
		// Missing database name.
		{"mongo:///mycollection", true},
		// Missing collection name.
		{"mongo://mydb/", true},
		// Passing id_field parameter.
		{"mongo://mydb/mycollection?id_field=foo", false},
		// Invalid parameter.
		{"mongo://mydb/mycollection?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		d, err := docstore.OpenCollection(ctx, test.URL)
		if d != nil {
			defer d.Close()
		}
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
