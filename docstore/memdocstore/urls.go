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
	"fmt"
	"net/url"
	"strings"
	"sync"

	"gocloud.dev/docstore"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, &URLOpener{})
}

// Scheme is the URL scheme memdocstore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "mem"

// URLOpener opens URLs like "mem://collection/_id".
//
// The URL's host is the name of the collection.
// The URL's path is used as the keyField.
//
// The following query parameters are supported:
//
//   - revision_field (optional): the name of the revision field.
//   - filename (optional): the filename to store the collection in.
type URLOpener struct {
	mu          sync.Mutex
	collections map[string]urlColl
}

type urlColl struct {
	keyName string
	coll    *docstore.Collection
}

// OpenCollectionURL opens a docstore.Collection based on u.
func (o *URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	q := u.Query()
	collName := u.Host
	if collName == "" {
		return nil, fmt.Errorf("open collection %v: empty collection name", u)
	}
	keyName := u.Path
	if strings.HasPrefix(keyName, "/") {
		keyName = keyName[1:]
	}
	if keyName == "" || strings.ContainsRune(keyName, '/') {
		return nil, fmt.Errorf("open collection %v: invalid key name %q (must be non-empty and have no slashes)", u, keyName)
	}

	options := &Options{
		RevisionField: q.Get("revision_field"),
		Filename:      q.Get("filename"),
		onClose: func() {
			o.mu.Lock()
			delete(o.collections, collName)
			o.mu.Unlock()
		},
	}
	q.Del("revision_field")
	q.Del("filename")
	for param := range q {
		return nil, fmt.Errorf("open collection %v: invalid query parameter %q", u, param)
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.collections == nil {
		o.collections = map[string]urlColl{}
	}
	ucoll, ok := o.collections[collName]
	if !ok {
		coll, err := OpenCollection(keyName, options)
		if err != nil {
			return nil, err
		}
		o.collections[collName] = urlColl{keyName, coll}
		return coll, nil
	}
	if ucoll.keyName != keyName {
		return nil, fmt.Errorf("open collection %v: key name %q does not equal existing key name %q",
			u, keyName, ucoll.keyName)
	}
	return ucoll.coll, nil
}
