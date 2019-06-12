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

package dynamodocstore_test

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"gocloud.dev/internal/docstore"
	"gocloud.dev/internal/docstore/dynamodocstore"
)

func ExampleOpenCollection() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// import _ "gocloud.dev/docstore/dynamodocstore"

	sess, err := session.NewSession()
	if err != nil {
		log.Fatal(err)
	}
	coll, err := dynamodocstore.OpenCollection(
		dynamodb.New(sess), "docstore-test", "partitionKeyField", "", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}

func Example_openCollectionFromURL() {
	// This example is used in https://gocloud.dev/howto/docstore.

	// import _ "gocloud.dev/docstore/dynamodocstore"

	// Variables set up elsewhere:
	ctx := context.Background()

	// docstore.OpenCollection creates a *docstore.Collection from a URL.
	coll, err := docstore.OpenCollection(ctx, "dynamodb://my-table?partition_key=name")
	if err != nil {
		log.Fatal(err)
	}
	defer coll.Close()
}
