// Copyright 2018 The Go Cloud Authors
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
package rdsmysql

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cloud/aws/rds"
	"github.com/google/go-cloud/wire"
	"github.com/opencensus-integrations/ocsql"
)

// Set is a Wire provider set that provides a *sql.DB given
// *Params and an HTTP client.
var Set = wire.NewSet(
	Open,
	wire.Value((*Options)(nil)),
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
}

type Options struct {
	// This is only a placeholder so far.
}

// Open opens an encrypted connection to an RDS MySQL database.
//
// The second return value is a Wire cleanup function that calls Close on the
// database and ignores the error.
func Open(ctx context.Context, provider CertPoolProvider, params *Params, opts *Options) (*sql.DB, func(), error) {
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
		tlsConfigName := fmt.Sprintf("github.com/google/go-cloud/mysql/rdsmysql/%d", tlsConfigNum)
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
	return ocsql.Wrap(mysql.MySQLDriver{}, ocsql.WithAllTraceOptions())
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
