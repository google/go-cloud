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

package firedocstore

import (
	"context"
	"net/url"
	"testing"

	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/testing/setup"
)

func TestCollectionNameFromURL(t *testing.T) {
	tests := []struct {
		URL          string
		WantErr      bool
		WantProject  string
		WantCollPath string
	}{
		{"firestore://proj/coll", false, "proj", "coll"},
		{"firestore://proj/coll/doc/subcoll", false, "proj", "coll/doc/subcoll"},
		{"firestore://proj/", true, "", ""},
		{"firestore:///coll", true, "", ""},
	}
	for _, test := range tests {
		u, err := url.Parse(test.URL)
		if err != nil {
			t.Fatal(err)
		}
		gotProj, gotCollPath, gotErr := collectionNameFromURL(u)
		if (gotErr != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, gotErr, test.WantErr)
		}
		if gotProj != test.WantProject || gotCollPath != test.WantCollPath {
			t.Errorf("%s: got project ID %s, collection path %s want project ID %s, collection path %s",
				test.URL, gotProj, gotCollPath, test.WantProject, test.WantCollPath)
		}
	}
}

func TestOpenCollection(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"firestore://myproject/mycoll?name_field=_id", false},
		// OK, hierarchical collection.
		{"firestore://myproject/mycoll/mydoc/subcoll?name_field=_id", false},
		// Missing project ID.
		{"firestore:///mycoll?name_field=_id", true},
		// Empty collection.
		{"firestore://myproject/", true},
		// Missing name field.
		{"firestore://myproject/mycoll", true},
		// Invalid param.
		{"firestore://myproject/mycoll?name_field=_id&param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := docstore.OpenCollection(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
