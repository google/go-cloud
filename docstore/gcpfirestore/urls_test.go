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

package gcpfirestore

import (
	"context"
	"testing"

	"gocloud.dev/docstore"
	"gocloud.dev/internal/testing/setup"
)

func TestOpenCollectionFromURL(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"firestore://projects/myproject/databases/(default)/documents/mycoll?name_field=_id", false},
		// OK, hierarchical collection.
		{"firestore://projects/myproject/databases/(default)/documents/mycoll/mydoc/subcoll?name_field=_id", false},
		// Missing project ID.
		{"firestore:///mycoll?name_field=_id", true},
		// Empty collection.
		{"firestore://projects/myproject/", true},
		// Missing name field.
		{"firestore://projects/myproject/databases/(default)/documents/mycoll", true},
		// Passing revision field.
		{"firestore://projects/myproject/databases/(default)/documents/mycoll?name_field=_id&revision_field=123", false},
		// Invalid param.
		{"firestore://projects/myproject/databases/(default)/documents/mycoll?name_field=_id&param=value", true},
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
