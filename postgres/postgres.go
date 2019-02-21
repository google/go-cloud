// Copyright 2018 The Go Cloud Development Kit Authors
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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
)

// A type that implements ConnectionURLOpener can open connection based on a URL.
// The opener must not modify the URL argument. OpenConnectionURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type ConnectionURLOpener interface {
	OpenConnectionURL(ctx context.Context, u *url.URL) (*sql.DB, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	schemes map[string]ConnectionURLOpener
}

// RegisterConnection registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterConnection panics.
func (mux *URLMux) RegisterConnection(scheme string, opener ConnectionURLOpener) {
	if mux.schemes == nil {
		mux.schemes = make(map[string]ConnectionURLOpener)
	} else if _, exists := mux.schemes[scheme]; exists {
		panic(fmt.Errorf("scheme %q already registered on mux", scheme))
	}
	mux.schemes[scheme] = opener
}

// OpenConnection calls OpenConnectionURL with the URL parsed from urlstr.
// OpenConnection is safe to call from multiple goroutines.
func (mux *URLMux) OpenConnection(ctx context.Context, urlstr string) (*sql.DB, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, fmt.Errorf("open connection: %v", err)
	}
	return mux.OpenConnectionURL(ctx, u)
}

// OpenConnectionURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenConnectionURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenConnectionURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	if u.Scheme == "" {
		return nil, fmt.Errorf("open connection %q: no scheme in URL", u)
	}
	var opener ConnectionURLOpener
	if mux != nil {
		opener = mux.schemes[u.Scheme]
	}
	if opener == nil {
		return nil, fmt.Errorf("open connection %q: no provider registered for %s", u, u.Scheme)
	}
	return opener.OpenConnectionURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenConnection.
//
// Driver packages can use this to register their ConnectionURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenConnection opens the bucket identified by the URL given. URL openers must be
// registered in the DefaultURLMux, which is typically done in driver
// packages' initialization.
//
// See the URLOpener documentation in provider-specific subpackages for more
// details on supported scheme(s) and URL parameter(s).
func OpenConnection(ctx context.Context, urlstr string) (*sql.DB, error) {
	return defaultURLMux.OpenConnection(ctx, urlstr)
}
