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
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/mongo"
	"gocloud.dev/docstore"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, new(defaultDialer))
}

// defaultDialer dials a default Mongo server based on the environment variable
// MONGO_SERVER_URL.
type defaultDialer struct {
	mongoServerURL string
	mu             sync.Mutex
	opener         *URLOpener
	err            error
}

func (o *defaultDialer) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	currentEnv := os.Getenv("MONGO_SERVER_URL")

	if currentEnv == "" {
		o.err = errors.New("MONGO_SERVER_URL environment variable is not set")
		return nil, fmt.Errorf("open collection %s: %v", u, o.err)
	}

	// If MONGO_SERVER_URL has been updated, then update o.opener as well
	if currentEnv != o.mongoServerURL {
		client, err := Dial(ctx, currentEnv)
		if err != nil {
			o.err = fmt.Errorf("failed to dial default Mongo server at %q: %v", currentEnv, err)
			return nil, fmt.Errorf("open collection %s: %v", u, o.err)
		}
		o.mongoServerURL = currentEnv
		o.opener = &URLOpener{Client: client}
	}
	return o.opener.OpenCollectionURL(ctx, u)
}

// Scheme is the URL scheme mongodocstore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "mongo"

// URLOpener opens URLs like "mongo://mydb/mycollection".
// See https://docs.mongodb.com/manual/reference/limits/#naming-restrictions for
// naming restrictions.
//
// The URL Host is used as the database name.
// The URL Path is used as the collection name.
//
// The following query parameters are supported:
//
//   - id_field (optional): the field name to use for the "_id" field.
type URLOpener struct {
	// A Client is a MongoDB client that performs operations on the db, must be
	// non-nil.
	Client *mongo.Client

	// Options specifies the options to pass to OpenCollection.
	Options Options
}

// OpenCollectionURL opens the Collection URL.
func (o *URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	q := u.Query()
	idField := q.Get("id_field")
	q.Del("id_field")
	o.Options.RevisionField = q.Get("revision_field")
	q.Del("revision_field")

	for param := range q {
		return nil, fmt.Errorf("open collection %s: invalid query parameter %q", u, param)
	}

	dbName := u.Host
	if dbName == "" {
		return nil, fmt.Errorf("open collection %s: URL must have a non-empty Host (database name)", u)
	}
	collName := strings.TrimPrefix(u.Path, "/")
	if collName == "" {
		return nil, fmt.Errorf("open collection %s: URL must have a non-empty Path (collection name)", u)
	}
	return OpenCollection(o.Client.Database(dbName).Collection(collName), idField, &o.Options)
}
