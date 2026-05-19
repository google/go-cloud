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
// # URLs
//
// For postgres.Open, awspostgres registers for the scheme "awspostgres".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://docs.aws.amazon.com/sdk-for-go/api/aws/session/.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// To use IAM authentication, omit the password from the URL:
//
//	awspostgres://iam-user@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:5432/mydb
//
// To specify an AWS profile or assume a role, add the following query parameters:
//
//   - aws_profile: the AWS shared config profile to use
//   - aws_role_arn: the ARN of the role to assume
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
	"net/http"
	"net/url"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/lib/pq"
	"gocloud.dev/aws/rds"
	"gocloud.dev/postgres"
)

// URLOpener opens RDS PostgreSQL URLs
// like "awspostgres://user:password@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:5432/mydb".
//
// To use IAM authentication, omit the password:
//
//	awspostgres://iam-user@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:5432/mydb
//
// To specify an AWS profile or assume a role, add the following query parameters:
//
//   - aws_profile: the AWS shared config profile to use
//   - aws_role_arn: the ARN of the role to assume
type URLOpener struct {
	// HTTPClient is the HTTP client used to fetch RDS certificates,
	// and IAM authentication tokens.
	HTTPClient *http.Client
	// CertSource specifies how the opener will obtain the RDS Certificate
	// Authority. If nil, it will use the default *rds.CertFetcher.
	CertSource rds.CertPoolProvider
	// TraceOpts contains options for OpenTelemetry.
	TraceOpts []otelsql.Option
}

// Scheme is the URL scheme awspostgres registers its URLOpener under on
// postgres.DefaultMux.
const Scheme = "awspostgres"

func init() {
	postgres.DefaultURLMux().RegisterPostgres(Scheme, &URLOpener{})
}

// OpenPostgresURL opens a new RDS database connection wrapped with OpenTelemetry instrumentation.
func (uo *URLOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	source := uo.CertSource
	if source == nil {
		source = &rds.CertFetcher{Client: uo.HTTPClient}
	}
	if u.Host == "" {
		return nil, fmt.Errorf("awspostgres: open: empty endpoint")
	}

	query := u.Query()
	for k := range query {
		// Forbid SSL-related parameters.
		if k == "sslmode" || k == "sslcert" || k == "sslkey" || k == "sslrootcert" {
			return nil, fmt.Errorf("awspostgres: open: parameter %q not allowed; sslmode must be disabled because the underlying dialer is already providing TLS", k)
		}
	}

	// If no password provided, assume it's AWS IAM authentication.
	var iam func(context.Context) (string, error)
	if _, ok := u.User.Password(); !ok {
		profile := query.Get("aws_profile")
		query.Del("aws_profile")
		var cfgOpts []func(*config.LoadOptions) error
		if uo.HTTPClient != nil {
			cfgOpts = append(cfgOpts, config.WithHTTPClient(uo.HTTPClient))
		}
		if profile != "" {
			cfgOpts = append(cfgOpts, config.WithSharedConfigProfile(profile))
		}
		cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
		if err != nil {
			return nil, fmt.Errorf("awspostgres: open: load AWS config: %v", err)
		}
		creds := cfg.Credentials
		if roleARN := query.Get("aws_role_arn"); roleARN != "" {
			creds = stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), roleARN)
			query.Del("aws_role_arn")
		}
		creds = aws.NewCredentialsCache(creds)
		iam = func(ctx context.Context) (string, error) {
			return auth.BuildAuthToken(ctx, u.Host, cfg.Region, u.User.Username(), creds)
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
		traceOpts: append([]otelsql.Option(nil), uo.TraceOpts...),
		iam:       iam,
	})
	return db, nil
}

type pqDriver struct {
	provider  rds.CertPoolProvider
	traceOpts []otelsql.Option
}

func (d pqDriver) Open(name string) (driver.Conn, error) {
	c, _ := d.OpenConnector(name)
	return c.Connect(context.Background())
}

func (d pqDriver) OpenConnector(name string) (driver.Connector, error) {
	return connector{provider: d.provider, pqConn: name + " sslmode=disable", traceOpts: d.traceOpts}, nil
}

type connector struct {
	provider  rds.CertPoolProvider
	pqConn    string
	traceOpts []otelsql.Option
	iam       func(context.Context) (string, error)
}

func (c connector) Connect(ctx context.Context) (driver.Conn, error) {
	connStr := c.pqConn
	if c.iam != nil {
		token, err := c.iam(ctx)
		if err != nil {
			return nil, fmt.Errorf("awspostgres: refresh auth token: %v", err)
		}
		// Parse the connection string URL, set the password to the IAM token.
		u, err := url.Parse(connStr)
		if err != nil {
			return nil, fmt.Errorf("awspostgres: parse connection string: %v", err)
		}
		u.User = url.UserPassword(u.User.Username(), token)
		connStr = u.String()
	}
	conn, err := pq.DialOpen(dialer{c.provider}, connStr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c connector) Driver() driver.Driver {
	return otelsql.WrapDriver(pqDriver{c.provider, c.traceOpts}, c.traceOpts...)
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
	err = crypt.Handshake()
	if err != nil {
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
