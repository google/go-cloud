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
// # URLs
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
	"net/http"
	"net/url"

	"github.com/XSAM/otelsql"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/go-sql-driver/mysql"
	"github.com/google/wire"
	"gocloud.dev/aws/rds"
	gcmysql "gocloud.dev/mysql"
)

// Set is a Wire provider set that provides a *sql.DB given
// *Params and an HTTP client.
var Set = wire.NewSet(
	wire.Struct(new(URLOpener), "CertSource", "HTTPClient"),
	rds.CertFetcherSet,
)

// URLOpener opens RDS MySQL URLs
// like "awsmysql://user:password@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:3306/mydb".
//
// To use IAM authentication, omit the password:
//
//	awsmysql://iam-user@myinstance.borkxyzzy.us-west-1.rds.amazonaws.com:3306/mydb
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

// Scheme is the URL scheme awsmysql registers its URLOpener under on
// mysql.DefaultMux.
const Scheme = "awsmysql"

func init() {
	gcmysql.DefaultURLMux().RegisterMySQL(Scheme, &URLOpener{})
}

// OpenMySQLURL opens a new RDS database connection wrapped with OpenTelemetry instrumentation.
func (uo *URLOpener) OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	source := uo.CertSource
	if source == nil {
		source = &rds.CertFetcher{Client: uo.HTTPClient}
	}
	if u.Host == "" {
		return nil, fmt.Errorf("open OpenMySQLURL: empty endpoint")
	}
	// If no password provided, assume it's AWS IAM authentication.
	// awsmysql://iam-user@host:port/dbname
	var iam func(context.Context) (string, error)
	if _, ok := u.User.Password(); !ok {
		var (
			q       = u.Query()
			profile = q.Get("aws_profile")
		)
		q.Del("aws_profile")
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithHTTPClient(uo.HTTPClient),    // Ignored if nil.
			config.WithSharedConfigProfile(profile)) // Ignored if empty.
		if err != nil {
			return nil, fmt.Errorf("open OpenMySQLURL: load AWS config: %v", err)
		}
		creds := cfg.Credentials
		if roleARN := q.Get("aws_role_arn"); roleARN != "" {
			// If a RoleARN is specified, replace the credentials with AssumeRole provider.
			creds = stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), roleARN)
			q.Del("aws_role_arn")
		}
		u.RawQuery = q.Encode()
		creds = aws.NewCredentialsCache(creds)
		iam = func(ctx context.Context) (string, error) {
			// BuildAuthToken is local-operation
			// and does not make network calls.
			// The credentials provider may make network calls,
			// but we are wrapped it with caching.
			return auth.BuildAuthToken(ctx, u.Host, cfg.Region, u.User.Username(), creds)
		}
	}
	cfg, err := gcmysql.ConfigFromURL(u)
	if err != nil {
		return nil, err
	}
	c := &connector{
		cfg:      cfg,
		iam:      iam,
		provider: source,

		sem:   make(chan struct{}, 1),
		ready: make(chan struct{}),
	}
	c.sem <- struct{}{}
	return otelsql.OpenDB(c, uo.TraceOpts...), nil
}

type connector struct {
	sem      chan struct{} // receive to acquire, send to release
	provider CertPoolProvider

	ready chan struct{} // closed after resolving dsn
	cfg   *mysql.Config
	iam   func(context.Context) (string, error)
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-c.sem:
		certPool, err := c.provider.RDSCertPool(ctx)
		if err != nil {
			c.sem <- struct{}{} // release
			return nil, fmt.Errorf("connect RDS: %v", err)
		}
		c.cfg.TLS = &tls.Config{RootCAs: certPool}
		close(c.ready)
		// Don't release sem: make it block forever, so this case won't be run again.
	case <-c.ready:
		// Already succeeded.
	case <-ctx.Done():
		return nil, fmt.Errorf("connect RDS: waiting for certificates: %v", ctx.Err())
	}
	cfg := c.cfg.Clone()
	if c.iam != nil {
		var err error
		if cfg.Passwd, err = c.iam(ctx); err != nil {
			return nil, fmt.Errorf("connect RDS: refresh auth token: %v", err)
		}
	}
	inner, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect RDS: create connector: %v", err)
	}
	return inner.Connect(ctx)
}

func (c *connector) Driver() driver.Driver {
	return mysql.MySQLDriver{}
}

// A CertPoolProvider obtains a certificate pool that contains the RDS CA certificate.
type CertPoolProvider = rds.CertPoolProvider

// CertFetcher pulls the RDS CA certificates from Amazon's servers. The zero
// value will fetch certificates using the default HTTP client.
type CertFetcher = rds.CertFetcher
