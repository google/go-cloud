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

// Package postgres provides functions to open PostgreSQL databases with OpenCensus instrumentation.
package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"

	"github.com/lib/pq"

	"contrib.go.opencensus.io/integrations/ocsql"
)

// Scheme is the URL scheme this package registers its URLOpener under on
// DefaultMux.
const Scheme = "postgres"

func init() {
	DefaultURLMux().RegisterPostgres(Scheme, &URLOpener{})
}

// URLOpener opens URLs like "postgres://" by using the underlying PostgreSQL driver.
// See https://godoc.org/github.com/lib/pq#hdr-Connection_String_Parameters for details.
type URLOpener struct{}

// OpenPostgresURL opens a new database connection wrapped with OpenCensus instrumentation.
func (*URLOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	for k, _ := range u.Query() {
		// Only permit parameters that do not conflict with other behavior.
		if k == "user" || k == "password" || k == "dbname" || k == "host" || k == "port" {
			return nil, fmt.Errorf("postgres: openPostgresURL: extra parameter %s not allowed", k)
		}
	}

	db, err := openWithUrl(u)
	return db, err
}

func openWithUrl(url *url.URL) (*sql.DB, error) {
	return sql.OpenDB(connector{dsn: url.String()}), nil
}

type connector struct {
	dsn string
}

func (c connector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.Driver().Open(c.dsn)
}

func (c connector) Driver() driver.Driver {
	return ocsql.Wrap(&pq.Driver{})
}

// A type that implements PostgresURLOpener can open connection based on a URL.
// The opener must not modify the URL argument. OpenPostgresURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type PostgresURLOpener interface {
	OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	schemes map[string]PostgresURLOpener
}

// RegisterPostgres registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterPostgres panics.
func (mux *URLMux) RegisterPostgres(scheme string, opener PostgresURLOpener) {
	if mux.schemes == nil {
		mux.schemes = make(map[string]PostgresURLOpener)
	} else if _, exists := mux.schemes[scheme]; exists {
		panic(fmt.Errorf("scheme %q already registered on mux", scheme))
	}
	mux.schemes[scheme] = opener
}

// OpenPostgres calls OpenPostgresURL with the URL parsed from urlstr.
// OpenPostgres is safe to call from multiple goroutines.
func (mux *URLMux) OpenPostgres(ctx context.Context, urlstr string) (*sql.DB, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, fmt.Errorf("open connection: %v", err)
	}
	return mux.OpenPostgresURL(ctx, u)
}

// OpenPostgresURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenPostgresURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	if u.Scheme == "" {
		return nil, fmt.Errorf("open connection %q: no scheme in URL", u)
	}
	var opener PostgresURLOpener
	if mux != nil {
		opener = mux.schemes[u.Scheme]
	}
	if opener == nil {
		return nil, fmt.Errorf("open connection %q: no provider registered for %s", u, u.Scheme)
	}
	return opener.OpenPostgresURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenPostgres.
//
// Driver packages can use this to register their PostgresURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// Open opens the bucket identified by the URL given. URL openers must be
// registered in the DefaultURLMux, which is typically done in driver
// packages' initialization.
//
// See the URLOpener documentation in provider-specific subpackages for more
// details on supported scheme(s) and URL parameter(s).
func Open(ctx context.Context, urlstr string) (*sql.DB, error) {
	return defaultURLMux.OpenPostgres(ctx, urlstr)
}
