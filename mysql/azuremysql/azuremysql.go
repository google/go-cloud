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
package azuremysql // import "gocloud.dev/mysql/azuremysql"

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-sql-driver/mysql"

	"gocloud.dev/azure/azuredb"
)

const (
	defaultPort    = 3306
	endpointSuffix = "mysql.database.azure.com"
)

// Params specifies how to connect to an Azure Database for MySQL.
type Params struct {
	// ServerName is the MYSQL instance name without domain suffix. Example: gocloud
	ServerName string
	// User is the database user to connect as.
	User string
	// Password is the database user password to use.
	Password string
	// Database is the MYSQL database name to connect to.
	Database string
	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// GetFQDN constructs the FQDN for Azure Database for MySQL.
func (p *Params) GetFQDN() string {
	fqdn := fmt.Sprintf("%s.%s:%v", p.ServerName, endpointSuffix, defaultPort)
	return fqdn
}

// Validate ensures all required parameters are set.
func (p *Params) Validate() error {
	if p.ServerName == "" || p.User == "" || p.Database == "" {
		return fmt.Errorf("Missing one or more required params; got servername=%q username=%q database=%q", p.ServerName, p.User, p.Database)
	}
	return nil
}

// Open opens an encrypted connection to an Azure Database for MySql database.
func Open(ctx context.Context, cp CertPoolProvider, params *Params) (*sql.DB, func(), error) {
	if e := params.Validate(); e != nil {
		return nil, nil, e
	}
	c := &connector{
		provider: cp,
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

	sem      chan struct{}    // receive to acquire, send to release
	provider CertPoolProvider // provides the CA certificate pool

	ready chan struct{} // closed after writing dsn
	dsn   string
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-c.sem:
		certPool, err := c.provider.GetCertPool(ctx)
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect Azure MySql: %v", err)
		}

		tlsConfigName := fmt.Sprintf("gocloud.dev/mysql/azuresql/%s", c.params.ServerName)
		err = mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{
			RootCAs: certPool,
		})
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect Azure MySql: register TLS: %v", err)
		}
		cfg := &mysql.Config{
			Net:                     "tcp",
			Addr:                    c.params.GetFQDN(),
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
		return nil, fmt.Errorf("connect Azure MySql: waiting for certificates: %v", ctx.Err())
	}
	return c.Driver().Open(c.dsn)
}

func (c *connector) Driver() driver.Driver {
	return ocsql.Wrap(mysql.MySQLDriver{}, c.params.TraceOpts...)
}

// A CertPoolProvider obtains a certificate pool that contains the Azure CA certificate.
type CertPoolProvider = azuredb.CertPoolProvider

// CertFetcher is a default CertPoolProvider that can fetch CA certificates from
// any publicly accessible URI or File.
type CertFetcher = azuredb.AzureCertFetcher
