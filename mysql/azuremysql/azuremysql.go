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

// Package azuremysql provides connections to Azure Database for MySql.
package azuremysql // import "gocloud.dev/mysql/azuremysql"

import (
	"io/ioutil"
	"crypto/x509"
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/go-sql-driver/mysql"
	"github.com/google/wire"

)

// https://docs.microsoft.com/en-us/azure/mysql/howto-configure-ssl

// Set is a Wire provider set that provides a *sql.DB given
// *Params and an HTTP client.
var Set = wire.NewSet(
	Open,	
)

const (
	defaultPort = 3306
	endpointSuffix = "mysql.database.azure.com"
)

// Params specifies how to connect to an Azure Database for MySql.
type Params struct {
	// ServerName is the MySql instance name without domain suffex. Example: gocloud
	ServerName string
	// Port is the TCP port for Azure MySql Server
	Port int64
	// User is the database user to connect as.
	User string
	// Password is the database user password to use.
	Password string
	// Database is the MySQL database name to connect to.
	Database string
	// CertFilePath
	CertFilePath string

	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}
// GetHost constructs the FQDN for Azure Database for MySql
func (p *Params) GetHost() string {
	return ""
}
func (p *Params) MakeConnectionString() string {
	var connectionString string
	username := fmt.Sprintf("%s@%s", p.User, p.ServerName)
	fqdn := fmt.Sprintf("%s.%s:%v", p.ServerName, endpointSuffix, defaultPort) 
	connectionString = fmt.Sprintf("%s:%s@tcp(%s)/%s?allowNativePasswords=true&tls=custom", username, p.Password, fqdn, p.Database)  
	return connectionString
}

func (p *Params) Validate() error {
	return nil
}

func Open(ctx context.Context, acf *AzureCertFetcher, params *Params) (*sql.DB, func(), error) {	
	if e := params.Validate(); e != nil {
		return nil, nil, e
	}
	c := &connector{		
		//provider: *acf,
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
	provider struct{}

	ready chan struct{} // closed after writing dsn
	dsn   string
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-c.sem:
		rootCertPool := x509.NewCertPool()
		pem, _ := ioutil.ReadFile(c.params.CertFilePath)
		if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
			return nil, fmt.Errorf("failed to append PEM")
		}
		mysql.RegisterTLSConfig("custom", &tls.Config{RootCAs: rootCertPool})

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

var tlsConfigCounter struct {
	mu sync.Mutex
	n  int
}
