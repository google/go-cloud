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

// Package awsmysql provides connections to AWS RDS MySQL instances.
//
// URLs
//
// For mysql.Open, awsmysql registers for the scheme "awsmysql".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://docs.aws.amazon.com/sdk-for-go/api/aws/session/.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// See https://gocloud.dev/concepts/urls/ for background information.
package awsmysql // import "gocloud.dev/mysql/awsmysql"

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"sync/atomic"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-sql-driver/mysql"
	"github.com/google/wire"
	"gocloud.dev/aws/rds"
	gcmysql "gocloud.dev/mysql"
)

// Set is a Wire provider set that provides a *sql.DB given
// *Params and an HTTP client.
var Set = wire.NewSet(
	wire.Struct(new(URLOpener), "CertSource"),
	rds.CertFetcherSet,
)

// URLOpener opens RDS MySQL URLs
// like "awsmysql://user:password@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:3306/mydb".
type URLOpener struct {
	// CertSource specifies how the opener will obtain the RDS Certificate
	// Authority. If nil, it will use the default *rds.CertFetcher.
	CertSource rds.CertPoolProvider
	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// Scheme is the URL scheme awsmysql registers its URLOpener under on
// mysql.DefaultMux.
const Scheme = "awsmysql"

func init() {
	gcmysql.DefaultURLMux().RegisterMySQL(Scheme, &URLOpener{})
}

// OpenMySQLURL opens a new RDS database connection wrapped with OpenCensus instrumentation.
func (uo *URLOpener) OpenMySQLURL(_ context.Context, u *url.URL) (*sql.DB, error) {
	source := uo.CertSource
	if source == nil {
		source = new(rds.CertFetcher)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("open RDS: empty endpoint")
	}

	cfg, err := gcmysql.ConfigFromURL(u)
	if err != nil {
		return nil, err
	}
	c := &connector{
		dsn: cfg.FormatDSN(),
		// Make a copy of TraceOpts to avoid caller modifying.
		traceOpts: append([]ocsql.TraceOption(nil), uo.TraceOpts...),
		provider:  source,

		sem:   make(chan struct{}, 1),
		ready: make(chan struct{}),
	}
	c.sem <- struct{}{}
	return sql.OpenDB(c), nil
}

type connector struct {
	traceOpts []ocsql.TraceOption

	sem      chan struct{} // receive to acquire, send to release
	provider CertPoolProvider

	ready chan struct{} // closed after resolving dsn
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
		tlsConfigName := fmt.Sprintf(
			"gocloud.dev/mysql/awsmysql/%d",
			atomic.AddUint32(&tlsConfigCounter, 1),
		)
		err = mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{
			RootCAs: certPool,
		})
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect RDS: register TLS: %v", err)
		}
		cfg, _ := mysql.ParseDSN(c.dsn)
		cfg.TLSConfig = tlsConfigName
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
	return ocsql.Wrap(mysql.MySQLDriver{}, c.traceOpts...)
}

var tlsConfigCounter uint32

// A CertPoolProvider obtains a certificate pool that contains the RDS CA certificate.
type CertPoolProvider = rds.CertPoolProvider

// CertFetcher pulls the RDS CA certificates from Amazon's servers. The zero
// value will fetch certificates using the default HTTP client.
type CertFetcher = rds.CertFetcher
