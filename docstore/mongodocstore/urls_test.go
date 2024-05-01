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
	"net/url"
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
		// Passing revision field.
		{"mongo://mydb/mycollection?id_field=foo&revision_field=123", false},
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

func TestDefaultDialerOpenCollectionURL(t *testing.T) {
	// Defer cleanup
	oldURLVal := os.Getenv("MONGO_SERVER_URL")
	defer os.Setenv("MONGO_SERVER_URL", oldURLVal)

	tests := []struct {
		name                  string
		currentMongoServerURL string
		currentWantErr        bool
		newMongoServerURL     string
		newWantErr            bool
	}{
		{
			name:                  "fail when MONGO_SERVER_URL is empty / unset",
			currentMongoServerURL: "",
			currentWantErr:        true,
			newMongoServerURL:     "",
			newWantErr:            true,
		},
		{
			name:                  "fail when updated MONGO_SERVER_URL is empty / unset",
			currentMongoServerURL: "mongodb://localhost",
			currentWantErr:        false,
			newMongoServerURL:     "",
			newWantErr:            true,
		},
		{
			name:                  "pass when MONGO_SERVER_URL is updated to new value",
			currentMongoServerURL: "mongodb://localhost",
			currentWantErr:        false,
			newMongoServerURL:     "mongodb://localhost:27017",
			newWantErr:            false,
		},
	}

	// Set starting conditions
	d := new(defaultDialer)
	ctx := context.Background()
	mongoURLString := "mongo://mydb/mycollection"
	u, err := url.Parse(mongoURLString)
	if err != nil {
		t.Error(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set MONGO_SERVER_URL
			os.Setenv("MONGO_SERVER_URL", test.currentMongoServerURL)
			_, err = d.OpenCollectionURL(ctx, u)
			if err != nil && !test.currentWantErr {
				t.Error(err)
			}

			// Update MONGO_SERVER_URL
			os.Setenv("MONGO_SERVER_URL", test.newMongoServerURL)
			_, err = d.OpenCollectionURL(ctx, u)
			if err != nil && !test.newWantErr {
				t.Error(err)
			}

			// Check if the MONGO_SERVER_URL was updated after rotation
			if !test.newWantErr {
				if d.mongoServerURL != test.newMongoServerURL {
					t.Errorf("expected updated MONGO_SERVER_URL to be set to: %s, but got: %s", test.newMongoServerURL, d.mongoServerURL)
				}
			}
		})
	}
}
