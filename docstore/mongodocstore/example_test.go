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

package mongodocstore_test

import (
	"context"
	"log"

	"gocloud.dev/docstore"
	"gocloud.dev/docstore/mongodocstore"
)

func ExampleOpenCollection() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()

	client, err := mongodocstore.Dial(ctx, "mongodb://my-host")
	if err != nil {
		log.Fatal(err)
	}
	mcoll := client.Database("my-db").Collection("my-coll")
	coll, err := mongodocstore.OpenCollection(mcoll, "userID", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func ExampleOpenCollectionWithIDFunc() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	type HighScore struct {
		Game   string
		Player string
	}

	client, err := mongodocstore.Dial(ctx, "mongodb://my-host")
	if err != nil {
		log.Fatal(err)
	}
	mcoll := client.Database("my-db").Collection("my-coll")

	// The name of a document is constructed from the Game and Player fields.
	nameFromDocument := func(doc docstore.Document) interface{} {
		hs := doc.(*HighScore)
		return hs.Game + "|" + hs.Player
	}

	coll, err := mongodocstore.OpenCollectionWithIDFunc(mcoll, nameFromDocument, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func Example_openCollectionFromURL() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// import _ "gocloud.dev/docstore/mongodocstore"

	// Variables set up elsewhere:
	ctx := context.Background()

	// docstore.OpenCollection creates a *docstore.Collection from a URL.
	// MongoDB requires a unique ID key in each document. If the default
	// key _id is not available, you'll need to provide a field name in the
	// id_field URL parameter.
	// Note that if you use structs to represent documents, you cannot have
	// a _id field, so you'll have to provide a custom field.

	coll, err := docstore.OpenCollection(ctx, "mongo://my-db/my-collection?id_field=userID")
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}
