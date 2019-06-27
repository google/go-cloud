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

// Package awspostgres provides connections to AWS RDS PostgreSQL instances.
//
// URLs
//
// For postgres.Open, awspostgres registers for the scheme "awspostgres".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://docs.aws.amazon.com/sdk-for-go/api/aws/session/.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// See https://gocloud.dev/concepts/urls/ for background information.
package awspostgres // import "gocloud.dev/postgres/awspostgres"

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

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/lib/pq"
	"gocloud.dev/aws/rds"
	"gocloud.dev/postgres"
)

// URLOpener opens RDS PostgreSQL URLs
// like "awspostgres://user:password@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:5432/mydb".
type URLOpener struct {
	// CertSource specifies how the opener will obtain the RDS Certificate
	// Authority. If nil, it will use the default *rds.CertFetcher.
	CertSource rds.CertPoolProvider
	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// Scheme is the URL scheme awspostgres registers its URLOpener under on
// postgres.DefaultMux.
const Scheme = "awspostgres"

func init() {
	postgres.DefaultURLMux().RegisterPostgres(Scheme, &URLOpener{})
}

// OpenPostgresURL opens a new RDS database connection wrapped with OpenCensus instrumentation.
func (uo *URLOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	source := uo.CertSource
	if source == nil {
		source = new(rds.CertFetcher)
	}

	query := u.Query()
	for k := range query {
		// Forbid SSL-related parameters.
		if k == "sslmode" || k == "sslcert" || k == "sslkey" || k == "sslrootcert" {
			return nil, fmt.Errorf("awspostgres: open: parameter %q not allowed; sslmode must be disabled because the underlying dialer is already providing TLS", k)
		}
	}
	// sslmode must be disabled because the underlying dialer is already providing TLS.
	query.Set("sslmode", "disable")

	u2 := new(url.URL)
	*u2 = *u
	u2.Scheme = "postgres"
	u2.RawQuery = query.Encode()
	db := sql.OpenDB(connector{
		provider:  source,
		pqConn:    u2.String(),
		traceOpts: append([]ocsql.TraceOption(nil), uo.TraceOpts...),
	})
	return db, nil
}

type pqDriver struct {
	provider  rds.CertPoolProvider
	traceOpts []ocsql.TraceOption
}

func (d pqDriver) Open(name string) (driver.Conn, error) {
	c, _ := d.OpenConnector(name)
	return c.Connect(context.Background())
}

func (d pqDriver) OpenConnector(name string) (driver.Connector, error) {
	return connector{d.provider, name + " sslmode=disable", d.traceOpts}, nil
}

type connector struct {
	provider  rds.CertPoolProvider
	pqConn    string
	traceOpts []ocsql.TraceOption
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	conn, err := pq.DialOpen(dialer{c.provider}, c.pqConn)
	if err != nil {
		return nil, err
	}
	return ocsql.WrapConn(conn, c.traceOpts...), nil
}

func (c connector) Driver() driver.Driver {
	return pqDriver{c.provider, c.traceOpts}
}

type dialer struct {
	provider rds.CertPoolProvider
}

func (d dialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("awspostgres: parse address: %v", err)
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
