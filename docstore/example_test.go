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

package docstore_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"gocloud.dev/docstore"
	"gocloud.dev/docstore/memdocstore"
	_ "gocloud.dev/docstore/mongodocstore"
	"gocloud.dev/gcerrors"
)

type Player struct {
	Name             string
	Score            int
	DocstoreRevision interface{}
}

func ExampleCollection_Actions_bulkWrite() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	var coll *docstore.Collection

	// Build an ActionList to create several new players, then execute it.
	// The actions may happen in any order.
	newPlayers := []string{"Pat", "Mel", "Fran"}
	actionList := coll.Actions()
	for _, p := range newPlayers {
		actionList.Create(&Player{Name: p, Score: 0})
	}
	if err := actionList.Do(ctx); err != nil {
		log.Fatal(err)
	}
}

func ExampleCollection_Actions_getAfterWrite() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	var coll *docstore.Collection

	// Add a document to the collection, then retrieve it.
	// Because both the Put and the Get refer to the same document,
	// they happen in order.
	got := Player{Name: "Pat"}
	err := coll.Actions().Put(&Player{Name: "Pat", Score: 88}).Get(&got).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(got.Name, got.Score)
}

func ExampleCollection_Update() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	var coll *docstore.Collection

	// Create a player.
	pat := &Player{Name: "Pat", Score: 0}
	if err := coll.Create(ctx, pat); err != nil {
		log.Fatal(err)
	}

	// Set the score to a new value.
	pat2 := &Player{Name: "Pat"}
	err := coll.Actions().Update(pat, docstore.Mods{"Score": 15}).Get(pat2).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Increment the score.
	err = coll.Actions().Update(pat, docstore.Mods{"Score": docstore.Increment(5)}).Get(pat2).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleOpenCollection() {
	ctx := context.Background()
	// Open a collection using the firedocstore package.
	// You will need to blank-import the package for this to work:
	//   import _ "gocloud.dev/docstore/firedocstore"
	coll, err := docstore.OpenCollection(ctx, "firestore://my-collection")
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	_ = coll // Use the collection.
}

func ExampleCollection_As() {
	// This example is specific to the mongodocstore implementation; it demonstrates
	// access to the underlying go.mongodb.org/mongo-driver/mongo.Collection.
	// You will need to blank-import the package for this to work:
	//   import _ "gocloud.dev/docstore/mongodocstore"

	// The types exposed for As by mongodocstore are documented in
	// https://godoc.org/gocloud.dev/docstore/mongodocstore#hdr-As

	// This URL will open the collection using default credentials.
	ctx := context.Background()
	coll, err := docstore.OpenCollection(ctx, "mongo://my-db/my-collection")
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	// Try to access and use the underlying mongo.Collection.
	var mcoll *mongo.Collection
	if coll.As(&mcoll) {
		fmt.Println(mcoll.Database())
	} else {
		log.Println("Unable to access mongo.Collection through Collection.As")
	}
}

func ExampleCollection_ErrorAs() {
	// This example is specific to the mongodocstore implementation; it demonstrates
	// access to the underlying go.mongodb.org/mongo-driver/mongo.Collection.
	// You will need to blank-import the package for this to work:
	//   import _ "gocloud.dev/docstore/mongodocstore"

	// The types exposed for As by mongodocstore are documented in
	// https://godoc.org/gocloud.dev/docstore/mongodocstore#hdr-As

	// This URL will open the collection using default credentials.
	ctx := context.Background()
	coll, err := docstore.OpenCollection(ctx, "mongo://my-db/my-collection")
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	doc := map[string]interface{}{"_id": "a"}
	if err := coll.Create(ctx, doc); err != nil {
		var bwe mongo.BulkWriteError
		if coll.ErrorAs(err, &bwe) {
			fmt.Println("got", bwe)
		} else {
			fmt.Println("could not convert error")
		}
	}
}

func ExampleQuery_Get() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	var coll *docstore.Collection

	// Ask for all players with scores at least 20.
	iter := coll.Query().Where("Score", ">=", 20).OrderBy("Score", docstore.Descending).Get(ctx)
	defer iter.Stop()

	// Query.Get returns an iterator. Call Next on it until io.EOF.
	for {
		var p Player
		err := iter.Next(ctx, &p)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		} else {
			fmt.Printf("%s: %d\n", p.Name, p.Score)
		}
	}
}

func ExampleQuery_Get_full() {
	ctx := context.Background()
	coll, err := memdocstore.OpenCollection("Name", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	// Add some documents to the collection.
	err = coll.Actions().
		Put(&Player{Name: "Pat", Score: 10}).
		Put(&Player{Name: "Mel", Score: 20}).
		Put(&Player{Name: "Fran", Score: 30}).
		Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Ask for all players with scores at least 20.
	iter := coll.Query().Where("Score", ">=", 20).OrderBy("Score", docstore.Descending).Get(ctx)
	defer iter.Stop()

	// Query.Get returns an iterator. Call Next on it until io.EOF.
	for {
		var p Player
		err := iter.Next(ctx, &p)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		} else {
			fmt.Printf("%s: %d\n", p.Name, p.Score)
		}
	}

	// Output:
	// Fran: 30
	// Mel: 20
}

func ExampleQuery_Delete() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	var coll *docstore.Collection

	// Delete all players with scores under 25.
	err := coll.Query().Where("Score", "<", 25).Delete(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleQuery_Delete_full() {
	ctx := context.Background()
	coll, err := memdocstore.OpenCollection("Name", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	// Add some documents to the collection.
	err = coll.Actions().
		Put(&Player{Name: "Pat", Score: 10}).
		Put(&Player{Name: "Mel", Score: 20}).
		Put(&Player{Name: "Fran", Score: 30}).
		Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Delete all players with scores over 25.
	err = coll.Query().Where("Score", ">", 25).Delete(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Print the remaining players.
	iter := coll.Query().OrderBy("Name", docstore.Ascending).Get(ctx)
	defer iter.Stop()
	for {
		var p Player
		err := iter.Next(ctx, &p)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		} else {
			fmt.Printf("%s: %d\n", p.Name, p.Score)
		}
	}

	// Output:
	// Mel: 20
	// Pat: 10
}

func ExampleQuery_Update() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()
	var coll *docstore.Collection

	// Increment a player's score if it is low.
	err := coll.Query().
		Where("Score", "<", 20).
		Update(ctx, docstore.Mods{"Score": docstore.Increment(15)})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleQuery_Update_full() {
	ctx := context.Background()
	coll, err := memdocstore.OpenCollection("Name", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	// Add some documents to the collection.
	err = coll.Actions().
		Put(&Player{Name: "Pat", Score: 10}).
		Put(&Player{Name: "Mel", Score: 20}).
		Put(&Player{Name: "Fran", Score: 30}).
		Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Increment a player's score if it is low.
	err = coll.Query().
		Where("Score", "<", 20).
		Update(ctx, docstore.Mods{"Score": docstore.Increment(15)})
	if err != nil {
		log.Fatal(err)
	}

	// Print the players.
	iter := coll.Query().OrderBy("Name", docstore.Ascending).Get(ctx)
	defer iter.Stop()
	for {
		var p Player
		err := iter.Next(ctx, &p)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		} else {
			fmt.Printf("%s: %d\n", p.Name, p.Score)
		}
	}

	// Output:
	// Fran: 30
	// Mel: 20
	// Pat: 25
}

func Example_optimisticLocking() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// Variables set up elsewhere:
	ctx := context.Background()

	coll, err := memdocstore.OpenCollection("Name", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()

	// Create a player.
	pat := &Player{Name: "Pat", Score: 7}
	if err := coll.Create(ctx, pat); err != nil {
		log.Fatal(err)
	}
	fmt.Println(pat) // memdocstore revisions are deterministic, so we can check the output.

	// Double a player's score. We cannot use Update to multiply, so we use optimistic
	// locking instead.

	// We may have to retry a few times; put a time limit on that.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		// Get the document.
		player := &Player{Name: "Pat"}
		if err := coll.Get(ctx, player); err != nil {
			log.Fatal(err)
		}
		// player.DocstoreRevision is set to the document's revision.

		// Modify the document locally.
		player.Score *= 2

		// Replace the document. player.DocstoreRevision will be checked against
		// the stored document's revision.
		err := coll.Replace(ctx, player)
		if err != nil {
			code := gcerrors.Code(err)
			// On FailedPrecondition or NotFound, try again.
			if code == gcerrors.FailedPrecondition || code == gcerrors.NotFound {
				continue
			}
			log.Fatal(err)
		}
		fmt.Println(player)
		break
	}

	// Output:
	// &{Pat 7 1}
	// &{Pat 14 2}
}
