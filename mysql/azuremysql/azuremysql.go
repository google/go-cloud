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

// Package azuremysql provides connections to Azure Database for MySQL.
// See https://docs.microsoft.com/en-us/azure/mysql.
//
// URLs
//
// For mysql.Open, azuremysql registers for the scheme "azuremysql".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// See https://gocloud.dev/concepts/urls/ for background information.
package azuremysql // import "gocloud.dev/mysql/azuremysql"

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
	"gocloud.dev/azure/azuredb"
	cdkmysql "gocloud.dev/mysql"
)

// URLOpener opens Azure MySQL URLs
// like "azuremysql://user:password@myinstance.mysql.database.azure.com/mydb".
type URLOpener struct {
	// CertSource specifies how the opener will obtain the Azure Certificate
	// Authority. If nil, it will use the default *azuredb.CertFetcher.
	CertSource azuredb.CertPoolProvider
	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// Scheme is the URL scheme azuremysql registers its URLOpener under on
// mysql.DefaultMux.
const Scheme = "azuremysql"

func init() {
	cdkmysql.DefaultURLMux().RegisterMySQL(Scheme, &URLOpener{})
}

// OpenMySQLURL opens an encrypted connection to an Azure MySQL database.
func (uo *URLOpener) OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	source := uo.CertSource
	if source == nil {
		source = new(azuredb.CertFetcher)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("open Azure database: empty endpoint")
	}
	password, _ := u.User.Password()
	c := &connector{
		addr:     u.Host,
		user:     u.User.Username(),
		password: password,
		dbName:   strings.TrimPrefix(u.Path, "/"),
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
	addr      string
	user      string
	password  string
	dbName    string
	traceOpts []ocsql.TraceOption

	sem      chan struct{}    // receive to acquire, send to release
	provider CertPoolProvider // provides the CA certificate pool

	ready chan struct{} // closed after writing dsn
	dsn   string
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-c.sem:
		certPool, err := c.provider.AzureCertPool(ctx)
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect Azure MySql: %v", err)
		}

		// TODO(light): Avoid global registry once https://github.com/go-sql-driver/mysql/issues/771 is fixed.
		tlsConfigCounter.mu.Lock()
		tlsConfigNum := tlsConfigCounter.n
		tlsConfigCounter.n++
		tlsConfigCounter.mu.Unlock()
		tlsConfigName := fmt.Sprintf("gocloud.dev/mysql/azuremysql/%d", tlsConfigNum)
		err = mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{
			RootCAs: certPool,
		})
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect Azure MySql: register TLS: %v", err)
		}
		cfg := &mysql.Config{
			Net:                     "tcp",
			Addr:                    c.addr,
			User:                    c.user,
			Passwd:                  c.password,
			TLSConfig:               tlsConfigName,
			AllowCleartextPasswords: true,
			AllowNativePasswords:    true,
			DBName:                  c.dbName,
		}
		c.dsn = cfg.FormatDSN()
		close(c.ready)
		// Don't release sem: make it block forever, so this case won't be run again.
	case <-c.ready:
		// Already succeeded.
	case <-ctx.Done():
		return nil, fmt.Errorf("connect Azure MySql: waiting for certificates: %v", ctx.Err())
	}
	return c.Driver().Open(c.dsn)
}

func (c *connector) Driver() driver.Driver {
	return ocsql.Wrap(mysql.MySQLDriver{}, c.traceOpts...)
}

var tlsConfigCounter struct {
	mu sync.Mutex
	n  int
}

// A CertPoolProvider obtains a certificate pool that contains the Azure CA certificate.
type CertPoolProvider = azuredb.CertPoolProvider

// CertFetcher is a default CertPoolProvider that can fetch CA certificates from
// any publicly accessible URI or File.
type CertFetcher = azuredb.CertFetcher
