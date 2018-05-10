// Copyright 2018 Google LLC
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
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/net/context/ctxhttp"
)

// caBundleURL is the URL to the public RDS Certificate Authority keys.
const caBundleURL = "https://s3.amazonaws.com/rds-downloads/rds-combined-ca-bundle.pem"

// FetchCertificates reads the RDS Certificate Authority keys from Amazon's servers.
func FetchCertificates(ctx context.Context, client *http.Client) ([]*x509.Certificate, error) {
	resp, err := ctxhttp.Get(ctx, client, caBundleURL)
	if err != nil {
		return nil, fmt.Errorf("fetch RDS certificates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch RDS certificates: HTTP %s", resp.Status)
	}
	pemData, err := ioutil.ReadAll(&io.LimitedReader{R: resp.Body, N: 1 << 20}) // limit to 1MiB
	if err != nil {
		return nil, fmt.Errorf("fetch RDS certificates: %v", err)
	}
	var certs []*x509.Certificate
	for len(pemData) > 0 {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("fetch RDS certificates: %v", err)
		}
		certs = append(certs, c)
	}
	return certs, nil
}

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

// Open opens an encrypted connection to an RDS database.
func Open(ctx context.Context, certPool *x509.CertPool, params *Params) (*sql.DB, error) {
	tlsConfigCounter.mu.Lock()
	tlsConfigNum := tlsConfigCounter.n
	tlsConfigCounter.n++
	tlsConfigCounter.mu.Unlock()
	tlsConfigName := fmt.Sprintf("github.com/google/go-cloud/mysql/rdsmysql/%d", tlsConfigNum)
	err := mysql.RegisterTLSConfig(tlsConfigName, &tls.Config{
		RootCAs: certPool,
	})
	if err != nil {
		return nil, fmt.Errorf("open RDS database: register TLS: %v", err)
	}
	cfg := &mysql.Config{
		Net:                     "tcp",
		Addr:                    params.Endpoint,
		User:                    params.User,
		Passwd:                  params.Password,
		TLSConfig:               tlsConfigName,
		AllowCleartextPasswords: true,
		DBName:                  params.Database,
	}
	return sql.OpenDB(connector(cfg.FormatDSN())), nil
}

var tlsConfigCounter struct {
	mu sync.Mutex
	n  int
}

type connector string

func (c connector) Connect(context.Context) (driver.Conn, error) {
	dsn := string(c)
	return mysql.MySQLDriver{}.Open(dsn)
}

func (c connector) Driver() driver.Driver {
	return mysql.MySQLDriver{}
}
