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

// Package mysql provides functions to open MySQL databases with OpenCensus instrumentation.
package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gocloud.dev/internal/openurl"

	"contrib.go.opencensus.io/integrations/ocsql"
)

// Scheme is the URL scheme this package registers its URLOpener under on
// DefaultMux.
const Scheme = "mysql"

func init() {
	DefaultURLMux().RegisterMySQL(Scheme, &URLOpener{})
}

// URLOpener opens URLs like "mysql://" by using the underlying MySQL driver.
// like "mysql://user:password@localhost:3306/mydb".
type URLOpener struct {
	TraceOpts []ocsql.TraceOption
}

// OpenMySQLURL opens a new database connection wrapped with OpenCensus instrumentation.
func (uo *URLOpener) OpenMySQLURL(_ context.Context, u *url.URL) (*sql.DB, error) {
	cfg, err := ConfigFromURL(u)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector{
		dsn:       cfg.FormatDSN(),
		traceOpts: append([]ocsql.TraceOption(nil), uo.TraceOpts...),
	}), nil
}

var netAddrRE = regexp.MustCompile(`^(.+)\((.+)\)$`)

// ConfigFromURL creates a mysql.Config from URL.
func ConfigFromURL(u *url.URL) (cfg *mysql.Config, err error) {
	dbName := strings.TrimPrefix(u.Path, "/")
	if u.RawQuery != "" {
		optDsn := fmt.Sprintf("/%s?%s", dbName, u.RawQuery)
		if cfg, err = mysql.ParseDSN(optDsn); err != nil {
			return nil, err
		}
	} else {
		cfg = mysql.NewConfig()
	}
	if matches := netAddrRE.FindStringSubmatch(u.Host); len(matches) == 3 {
		cfg.Net = matches[1]
		cfg.Addr = matches[2]
	} else {
		cfg.Net = "tcp"
		cfg.Addr = u.Host
	}
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.DBName = dbName
	cfg.AllowCleartextPasswords = true
	cfg.AllowNativePasswords = true
	return cfg, nil
}

type connector struct {
	dsn       string
	traceOpts []ocsql.TraceOption
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	return c.Driver().Open(c.dsn)
}

func (c connector) Driver() driver.Driver {
	return ocsql.Wrap(mysql.MySQLDriver{}, c.traceOpts...)
}

// MySQLURLOpener implements MySQLURLOpener and can open connections based on a URL.
// The opener must not modify the URL argument. OpenMySQLURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type MySQLURLOpener interface {
	OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	schemes openurl.SchemeMap
}

// RegisterMySQL registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterMySQL panics.
func (mux *URLMux) RegisterMySQL(scheme string, opener MySQLURLOpener) {
	mux.schemes.Register("mysql", "DB", scheme, opener)
}

// OpenMySQL calls OpenMySQLURL with the URL parsed from urlstr.
// OpenMySQL is safe to call from multiple goroutines.
func (mux *URLMux) OpenMySQL(ctx context.Context, urlstr string) (*sql.DB, error) {
	opener, u, err := mux.schemes.FromString("DB", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(MySQLURLOpener).OpenMySQLURL(ctx, u)
}

// OpenMySQLURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenMySQLURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	opener, err := mux.schemes.FromURL("DB", u)
	if err != nil {
		return nil, err
	}
	return opener.(MySQLURLOpener).OpenMySQLURL(ctx, u)
}

var defaultURLMux = new(URLMux)

// DefaultURLMux returns the URLMux used by OpenMySql.
//
// Driver packages can use this to register their MySQLURLOpener on the mux.
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
	return defaultURLMux.OpenMySQL(ctx, urlstr)
}
