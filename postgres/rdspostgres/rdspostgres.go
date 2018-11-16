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

// Package rdspostgres provides connections to AWS RDS PostgreSQL instances.
package rdspostgres

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/google/go-cloud/aws/rds"
	"github.com/lib/pq"
)

// Params specifies how to connect to an RDS database.
type Params struct {
	// Endpoint is the host/port of the RDS database, like
	// "myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:5432".
	// If no port is given, then 5432 is assumed.
	Endpoint string

	// User is the database user to connect as.
	User string
	// Password is the database user password to use.
	Password string
	// Database is the PostgreSQL database name to connect to.
	Database string

	// Values sets additional parameters, as documented in
	// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS.
	Values url.Values
}

// Open opens an encrypted connection to an RDS database.
//
// The second return value is a Wire cleanup function that calls Close on the
// database and ignores the error.
func Open(ctx context.Context, provider rds.CertPoolProvider, params *Params) (*sql.DB, func(), error) {
	vals := make(url.Values)
	for k, v := range params.Values {
		// Only permit parameters that do not conflict with other behavior.
		if k == "user" || k == "password" || k == "dbname" || k == "host" || k == "port" || k == "sslmode" || k == "sslcert" || k == "sslkey" || k == "sslrootcert" {
			return nil, nil, fmt.Errorf("rdspostgres: open: extra parameter %s not allowed; use Params fields instead", k)
		}
		vals[k] = v
	}
	vals.Set("sslmode", "disable")

	var user *url.Userinfo
	if params.User != "" && params.Password != "" {
		if params.Password != "" {
			user = url.UserPassword(params.User, params.Password)
		} else {
			user = url.User(params.User)
		}
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     user,
		Host:     params.Endpoint,
		Path:     "/" + params.Database,
		RawQuery: vals.Encode(),
	}
	db := sql.OpenDB(connector{
		provider: provider,
		pqConn:   u.String(),
	})
	return db, func() { db.Close() }, nil
}

type pqDriver struct {
	provider rds.CertPoolProvider
}

func (d pqDriver) Open(name string) (driver.Conn, error) {
	c, _ := d.OpenConnector(name)
	return c.Connect(context.Background())
}

func (d pqDriver) OpenConnector(name string) (driver.Connector, error) {
	return connector{d.provider, name + " sslmode=disable"}, nil
}

type connector struct {
	provider rds.CertPoolProvider
	pqConn   string
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	return pq.DialOpen(dialer{c.provider}, c.pqConn)
}

func (c connector) Driver() driver.Driver {
	return pqDriver{c.provider}
}

type dialer struct {
	provider rds.CertPoolProvider
}

func (d dialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("rdspostgres: parse address: %v", err)
	}
	certPool, err := d.provider.RDSCertPool(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := new(net.Dialer).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	// Write the PostgreSQL SSLRequest message described in
	// https://www.postgresql.org/docs/11/protocol-message-formats.html
	// to upgrade to a TLS connection.
	_, err = conn.Write([]byte{
		// Message length (Int32), including message length.
		0x00, 0x00, 0x00, 0x08,
		// Magic number: 80877103.
		0x04, 0xd2, 0x16, 0x2f,
	})
	if err != nil {
		return nil, err
	}
	// Server must respond back with 'S'.
	var readBuf [1]byte
	if _, err := io.ReadFull(conn, readBuf[:]); err != nil {
		return nil, err
	}
	if readBuf[0] != 'S' {
		return nil, pq.ErrSSLNotSupported
	}

	// Begin TLS communication.
	crypt := tls.Client(conn, &tls.Config{
		ServerName:    host,
		RootCAs:       certPool,
		Renegotiation: tls.RenegotiateFreelyAsClient,
	})
	if err := crypt.Handshake(); err != nil {
		return nil, err
	}
	return crypt, nil
}

func (d dialer) Dial(network, address string) (net.Conn, error) {
	return d.dial(context.Background(), network, address)
}

func (d dialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	conn, err := d.dial(ctx, network, address)
	cancel()
	return conn, err
}
