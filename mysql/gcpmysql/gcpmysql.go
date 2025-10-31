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

// Package gcpmysql provides connections to managed MySQL Cloud SQL instances.
// See https://cloud.google.com/sql/docs/mysql/ for more information.
//
// # URLs
//
// For mysql.Open, gcpmysql registers for the scheme "gcpmysql".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// See https://gocloud.dev/concepts/urls/ for background information.
package gcpmysql // import "gocloud.dev/mysql/gcpmysql"

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/XSAM/otelsql"
	"github.com/go-sql-driver/mysql"
	"gocloud.dev/gcp"
	"gocloud.dev/gcp/cloudsql"
	cdkmysql "gocloud.dev/mysql"
)

// Scheme is the URL scheme gcpmysql registers its URLOpener under on
// mysql.DefaultMux.
const Scheme = "gcpmysql"

func init() {
	cdkmysql.DefaultURLMux().RegisterMySQL(Scheme, new(lazyCredsOpener))
}

// lazyCredsOpener obtains Application Default Credentials on the first call
// to OpenMySQLURL.
type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	o.init.Do(func() {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			o.err = err
			return
		}
		client, err := gcp.NewHTTPClient(gcp.DefaultTransport(), creds.TokenSource)
		if err != nil {
			o.err = err
			return
		}
		certSource := cloudsql.NewCertSource(client)
		o.opener = &URLOpener{CertSource: certSource}
	})
	if o.err != nil {
		return nil, fmt.Errorf("gcpmysql open %v: %v", u, o.err)
	}
	return o.opener.OpenMySQLURL(ctx, u)
}

// URLOpener opens Cloud MySQL URLs like
// "gcpmysql://user:password@project/region/instance/dbname".
type URLOpener struct {
	// CertSource specifies how the opener will obtain authentication information.
	// CertSource must not be nil.
	CertSource proxy.CertSource

	// TraceOpts contains options for OpenTelemetry.
	TraceOpts []otelsql.Option
}

// OpenMySQLURL opens a new GCP database connection wrapped with OpenTelemetry instrumentation.
func (uo *URLOpener) OpenMySQLURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	if uo.CertSource == nil {
		return nil, fmt.Errorf("gcpmysql: URLOpener CertSource is nil")
	}
	var (
		client   = &proxy.Client{Certs: uo.CertSource, Port: 3307}
		cfg, err = configFromURL(u)
	)
	if err != nil {
		return nil, fmt.Errorf("gcpmysql: open config %v", err)
	}
	cfg.DialFunc = func(ctx context.Context, _, addr string) (net.Conn, error) {
		// MySQL driver's addr is in the form "[host]:3306" after normalized.
		// https://github.com/go-sql-driver/mysql/blob/76c00e35a8d48f8f70f0e7dffe584692bd3fa612/dsn.go#L193-L195
		instance, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		return client.DialContext(ctx, instance)
	}
	c, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("gcpmysql: open connector %v", err)
	}
	return otelsql.OpenDB(c, uo.TraceOpts...), nil
}

func configFromURL(u *url.URL) (*mysql.Config, error) {
	instance, dbName, err := instanceFromURL(u)
	if err != nil {
		return nil, err
	}

	var cfg *mysql.Config
	switch {
	case len(u.RawQuery) > 0:
		optDsn := fmt.Sprintf("/%s?%s", dbName, u.RawQuery)
		if cfg, err = mysql.ParseDSN(optDsn); err != nil {
			return nil, err
		}
	default:
		cfg = mysql.NewConfig()
	}

	password, _ := u.User.Password()

	cfg.AllowNativePasswords = true
	cfg.Net = "tcp"
	cfg.Addr = instance
	cfg.User = u.User.Username()
	cfg.Passwd = password
	cfg.DBName = dbName

	return cfg, nil
}

func instanceFromURL(u *url.URL) (instance, db string, _ error) {
	path := u.Host + u.Path // everything after scheme but before query or fragment
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 4 {
		return "", "", fmt.Errorf("%s is not in the form project/region/instance/dbname", path)
	}
	for _, part := range parts {
		if part == "" {
			return "", "", fmt.Errorf("%s is not in the form project/region/instance/dbname", path)
		}
	}
	return parts[0] + ":" + parts[1] + ":" + parts[2], parts[3], nil
}
