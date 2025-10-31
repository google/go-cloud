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
// # URLs
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

	"github.com/XSAM/otelsql"
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
	// TraceOpts contains options for OpenTelemetry.
	TraceOpts []otelsql.Option
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
		cfg: &mysql.Config{
			Net:                     "tcp",
			Addr:                    u.Host,
			User:                    u.User.Username(),
			Passwd:                  password,
			AllowCleartextPasswords: true,
			AllowNativePasswords:    true,
			DBName:                  strings.TrimPrefix(u.Path, "/"),
		},
		provider: source,
		sem:      make(chan struct{}, 1),
		ready:    make(chan struct{}),
	}
	c.sem <- struct{}{}
	return otelsql.OpenDB(c, uo.TraceOpts...), nil
}

type connector struct {
	sem      chan struct{}    // receive to acquire, send to release
	provider CertPoolProvider // provides the CA certificate pool
	ready    chan struct{}    // closed after fetching certs successfully
	cfg      *mysql.Config
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-c.sem:
		certPool, err := c.provider.AzureCertPool(ctx)
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect Azure MySql: %v", err)
		}
		c.cfg.TLS = &tls.Config{RootCAs: certPool}
		close(c.ready)
		// Don't release sem: make it block forever, so this case won't be run again.
	case <-c.ready:
		// Already succeeded.
	case <-ctx.Done():
		return nil, fmt.Errorf("connect Azure MySql: waiting for certificates: %v", ctx.Err())
	}
	inner, err := mysql.NewConnector(c.cfg)
	if err != nil {
		return nil, fmt.Errorf("connect Azure MySql: create connector: %v", err)
	}
	return inner.Connect(ctx)
}

func (c *connector) Driver() driver.Driver {
	return mysql.MySQLDriver{}
}

var tlsConfigCounter uint32

// A CertPoolProvider obtains a certificate pool that contains the Azure CA certificate.
type CertPoolProvider = azuredb.CertPoolProvider

// CertFetcher is a default CertPoolProvider that can fetch CA certificates from
// any publicly accessible URI or File.
type CertFetcher = azuredb.CertFetcher
