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

package firedocstore_test

import (
	"context"
	"log"

	"gocloud.dev/docstore"
	"gocloud.dev/docstore/firedocstore"
	"gocloud.dev/gcp"
)

func ExampleOpenCollection() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()

	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client, _, err := firedocstore.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}
	resourceID := firedocstore.CollectionResourceID("my-project", "my-collection")
	coll, err := firedocstore.OpenCollection(client, resourceID, "userID", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func ExampleOpenCollectionWithNameFunc() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	type HighScore struct {
		Game   string
		Player string
	}

	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		log.Fatal(err)
	}
	client, _, err := firedocstore.Dial(ctx, creds.TokenSource)
	if err != nil {
		log.Fatal(err)
	}

	// The name of a document is constructed from the Game and Player fields.
	nameFromDocument := func(doc docstore.Document) string {
		hs := doc.(*HighScore)
		return hs.Game + "|" + hs.Player
	}

	resourceID := firedocstore.CollectionResourceID("my-project", "my-collection")
	coll, err := firedocstore.OpenCollectionWithNameFunc(client, resourceID, nameFromDocument, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func Example_openCollectionFromURL() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// import _ "gocloud.dev/docstore/firedocstore"

	// Variables set up elsewhere:
	ctx := context.Background()

	// docstore.OpenCollection creates a *docstore.Collection from a URL.
	const url = "firestore://projects/my-project/databases/(default)/documents/my-collection?name_field=userID"
	coll, err := docstore.OpenCollection(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}
