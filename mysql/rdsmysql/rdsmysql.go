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

// Package rdsmysql provides connections to AWS RDS MySQL instances.
//
// URLs
//
// For mysql.Open, rdsmysql registers for the scheme "rdsmysql".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://docs.aws.amazon.com/sdk-for-go/api/aws/session/.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// See https://gocloud.dev/concepts/urls/ for background information.
package rdsmysql // import "gocloud.dev/mysql/rdsmysql"

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-sql-driver/mysql"
	"github.com/google/wire"
	"gocloud.dev/aws/rds"
	gcmysql "gocloud.dev/mysql"
)

// Set is a Wire provider set that provides a *sql.DB given
// *Params and an HTTP client.
var Set = wire.NewSet(
	Open,
	rds.CertFetcherSet,
)

// Params specifies how to connect to an RDS database.
type Params struct {
	// Endpoint is the host/port of the RDS database, like
	// "myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:3306".
	// If no port is given, then 3306 is assumed.
	Endpoint string
	// User is the database user to connect as.
	User string
	// Password is the database user password to use.
	Password string
	// Database is the MySQL database name to connect to.
	Database string

	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// URLOpener opens RDS MySQL URLs
// like "rdsmysql://user:password@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:3306/mydb".
type URLOpener struct {
	// CertSource specifies how the opener will obtain authentication information.
	// CertSource must not be nil.
	CertSource rds.CertPoolProvider
	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// Scheme is the URL scheme rdsmysql registers its URLOpener under on
// mysql.DefaultMux.
const Scheme = "rdsmysql"

func init() {
	gcmysql.DefaultURLMux().RegisterMySQL(Scheme, &URLOpener{})
}

// OpenMySQLURL opens a new RDS database connection wrapped with OpenCensus instrumentation.
func (uo *URLOpener) OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	if uo.CertSource == nil {
		return nil, fmt.Errorf("rdsmysql: URLOpener CertSource is nil")
	}
	password, _ := u.User.Password()
	params := &Params{
		Endpoint:  u.Host,
		User:      u.User.Username(),
		Password:  password,
		Database:  strings.TrimPrefix(u.Path, "/"),
		TraceOpts: uo.TraceOpts,
	}
	db, _, err := Open(ctx, uo.CertSource, params)
	return db, err
}

// Open opens an encrypted connection to an RDS MySQL database.
//
// The second return value is a Wire cleanup function that calls Close on the
// database and ignores the error.
func Open(ctx context.Context, provider CertPoolProvider, params *Params) (*sql.DB, func(), error) {
	if params.Endpoint == "" {
		return nil, nil, fmt.Errorf("open RDS: endpoint empty")
	}
	c := &connector{
		provider: provider,
		params:   *params,
		sem:      make(chan struct{}, 1),
		ready:    make(chan struct{}),
	}
	c.sem <- struct{}{}
	// Make a copy of TraceOpts to avoid caller modifying.
	c.params.TraceOpts = append([]ocsql.TraceOption(nil), c.params.TraceOpts...)

	db := sql.OpenDB(c)
	return db, func() { db.Close() }, nil
}

type connector struct {
	params Params

	sem      chan struct{} // receive to acquire, send to release
	provider CertPoolProvider

	ready chan struct{} // closed after writing dsn
	dsn   string
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-c.sem:
		certPool, err := c.provider.RDSCertPool(ctx)
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect RDS: %v", err)
		}
		// TODO(light): Avoid global registry once https://github.com/go-sql-driver/mysql/issues/771 is fixed.
		tlsConfigCounter.mu.Lock()
		tlsConfigNum := tlsConfigCounter.n
		tlsConfigCounter.n++
		tlsConfigCounter.mu.Unlock()
		tlsConfigName := fmt.Sprintf("gocloud.dev/mysql/rdsmysql/%d", tlsConfigNum)
		err = mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{
			RootCAs: certPool,
		})
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect RDS: register TLS: %v", err)
		}
		cfg := &mysql.Config{
			Net:                     "tcp",
			Addr:                    c.params.Endpoint,
			User:                    c.params.User,
			Passwd:                  c.params.Password,
			TLSConfig:               tlsConfigName,
			AllowCleartextPasswords: true,
			AllowNativePasswords:    true,
			DBName:                  c.params.Database,
		}
		c.dsn = cfg.FormatDSN()
		close(c.ready)
		// Don't release sem: make it block forever, so this case won't be run again.
	case <-c.ready:
		// Already succeeded.
	case <-ctx.Done():
		return nil, fmt.Errorf("connect RDS: waiting for certificates: %v", ctx.Err())
	}
	return c.Driver().Open(c.dsn)
}

func (c *connector) Driver() driver.Driver {
	return ocsql.Wrap(mysql.MySQLDriver{}, c.params.TraceOpts...)
}

var tlsConfigCounter struct {
	mu sync.Mutex
	n  int
}

// A CertPoolProvider obtains a certificate pool that contains the RDS CA certificate.
type CertPoolProvider = rds.CertPoolProvider

// CertFetcher pulls the RDS CA certificates from Amazon's servers. The zero
// value will fetch certificates using the default HTTP client.
type CertFetcher = rds.CertFetcher
