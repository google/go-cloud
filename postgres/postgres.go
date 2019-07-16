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
	"net/url"

	"github.com/lib/pq"
	"gocloud.dev/internal/openurl"

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
	u2 := new(url.URL)
	*u2 = *u
	u2.Scheme = "postgres"
	return sql.OpenDB(connector{dsn: u2.String()}), nil
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
	schemes openurl.SchemeMap
}

// RegisterPostgres registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterPostgres panics.
func (mux *URLMux) RegisterPostgres(scheme string, opener PostgresURLOpener) {
	mux.schemes.Register("postgres", "DB", scheme, opener)
}

// OpenPostgres calls OpenPostgresURL with the URL parsed from urlstr.
// OpenPostgres is safe to call from multiple goroutines.
func (mux *URLMux) OpenPostgres(ctx context.Context, urlstr string) (*sql.DB, error) {
	opener, u, err := mux.schemes.FromString("DB", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(PostgresURLOpener).OpenPostgresURL(ctx, u)
}

// OpenPostgresURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenPostgresURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	opener, err := mux.schemes.FromURL("DB", u)
	if err != nil {
		return nil, err
	}
	return opener.(PostgresURLOpener).OpenPostgresURL(ctx, u)
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
// See the URLOpener documentation in driver subpackages for more
// details on supported scheme(s) and URL parameter(s).
func Open(ctx context.Context, urlstr string) (*sql.DB, error) {
	return defaultURLMux.OpenPostgres(ctx, urlstr)
}
