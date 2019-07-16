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

package docstore

import (
	"context"
	"net/url"

	"gocloud.dev/internal/openurl"
)

// CollectionURLOpener opens a collection of documents based on a URL.
// The opener must not modify the URL argument. It must be safe to call from
// multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type CollectionURLOpener interface {
	OpenCollectionURL(ctx context.Context, u *url.URL) (*Collection, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs against
// a set of registered schemes and calls the opener that matches the URL's
// scheme. See https://gocloud.dev/concepts/urls/ for more information.
//
// The zero value is a multiplexer with no registered scheme.
type URLMux struct {
	schemes openurl.SchemeMap
}

// CollectionSchemes returns a sorted slice of the registered Collection schemes.
func (mux *URLMux) CollectionSchemes() []string { return mux.schemes.Schemes() }

// ValidCollectionScheme returns true iff scheme has been registered for Collections.
func (mux *URLMux) ValidCollectionScheme(scheme string) bool { return mux.schemes.ValidScheme(scheme) }

// RegisterCollection registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterCollection panics.
func (mux *URLMux) RegisterCollection(scheme string, opener CollectionURLOpener) {
	mux.schemes.Register("docstore", "Collection", scheme, opener)
}

// OpenCollection calls OpenCollectionURL with the URL parsed from urlstr.
// OpenCollection is safe to call from multiple goroutines.
func (mux *URLMux) OpenCollection(ctx context.Context, urlstr string) (*Collection, error) {
	opener, u, err := mux.schemes.FromString("Collection", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(CollectionURLOpener).OpenCollectionURL(ctx, u)
}

// OpenCollectionURL dispatches the URL to the opener that is registered with
// the URL's scheme. OpenCollectionURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenCollectionURL(ctx context.Context, u *url.URL) (*Collection, error) {
	opener, err := mux.schemes.FromURL("Collection", u)
	if err != nil {
		return nil, err
	}
	return opener.(CollectionURLOpener).OpenCollectionURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenCollection.
//
// Driver packages can use this to register their CollectionURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenCollection opens the collection identified by the URL given.
// See the URLOpener documentation in driver subpackages for details
// on supported URL formats, and https://gocloud.dev/concepts/urls/ for more
// information.
func OpenCollection(ctx context.Context, urlstr string) (*Collection, error) {
	return defaultURLMux.OpenCollection(ctx, urlstr)
}
