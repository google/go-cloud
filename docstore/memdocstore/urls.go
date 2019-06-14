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

	"gocloud.dev/docstore"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, &URLOpener{})
}

// Scheme is the URL scheme memdocstore registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "mem"

// URLOpener opens URLs like "mem://_id".
//
// The URL's host is used as the keyField.
// The URL's path is ignored.
//
// No query parameters are supported.
type URLOpener struct{}

// OpenCollectionURL opens a docstore.Collection based on u.
func (*URLOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open collection %v: invalid query parameter %q", u, param)
	}
	return OpenCollection(u.Host, nil)
}
