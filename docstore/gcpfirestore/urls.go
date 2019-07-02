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
	"errors"
	"fmt"
	"net/url"
	"path"
	"sync"

	vkit "cloud.google.com/go/firestore/apiv1"
	"gocloud.dev/docstore"
	"gocloud.dev/gcp"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, &lazyCredsOpener{})
}

type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	o.init.Do(func() {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			o.err = err
			return
		}
		client, _, err := Dial(ctx, creds.TokenSource)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open collection %s: %v", u, o.err)
	}
	return o.opener.OpenCollectionURL(ctx, u)
}

// Scheme is the URL scheme firestore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "firestore"

// URLOpener opens firestore URLs like
// "firestore://projects/myproject/databases/(default)/documents/mycollection?name_field=myID".
//
// See https://firebase.google.com/docs/firestore/data-model for more details.
//
// The following query parameters are supported:
//
//   - name_field (required): gcpfirestore requires that a single string field,
// name_field, be designated the primary key. Its values must be unique over all
// documents in the collection, and the primary key must be provided to retrieve
// a document.
type URLOpener struct {
	// Client must be set to a non-nil client authenticated with Cloud Firestore
	// scope or equivalent.
	Client *vkit.Client
}

// OpenCollectionURL opens a docstore.Collection based on u.
func (o *URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	q := u.Query()
	options := &Options{
		RevisionField: q.Get("revision_field"),
	}

	nameField := q.Get("name_field")
	if nameField == "" {
		return nil, errors.New("open collection %s: name_field is required to open a collection")
	}
	q.Del("name_field")
	q.Del("revision_field")
	for param := range q {
		return nil, fmt.Errorf("open collection %s: invalid query parameter %q", u, param)
	}
	collResourceID := path.Join(u.Host, u.Path)
	return OpenCollection(o.Client, collResourceID, nameField, options)
}
