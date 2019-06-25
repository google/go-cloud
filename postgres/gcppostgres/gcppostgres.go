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

// Package gcppostgres provides connections to managed PostgreSQL Cloud SQL instances.
// See https://cloud.google.com/sql/docs/postgres/ for more information.
//
// URLs
//
// For postgres.Open, gcppostgres registers for the scheme "gcppostgres".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// See https://gocloud.dev/concepts/urls/ for background information.
package gcppostgres // import "gocloud.dev/postgres/gcppostgres"

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/lib/pq"
	"gocloud.dev/gcp"
	"gocloud.dev/gcp/cloudsql"
	"gocloud.dev/postgres"
)

// Scheme is the URL scheme gcppostgres registers its URLOpener under on
// postgres.DefaultMux.
const Scheme = "gcppostgres"

func init() {
	postgres.DefaultURLMux().RegisterPostgres(Scheme, new(lazyCredsOpener))
}

// lazyCredsOpener obtains Application Default Credentials on the first call
// to OpenPostgresURL.
type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
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
		return nil, fmt.Errorf("gcppostgres open %v: %v", u, o.err)
	}
	return o.opener.OpenPostgresURL(ctx, u)
}

// URLOpener opens GCP PostgreSQL URLs
// like "gcppostgres://user:password@myproject/us-central1/instanceId/mydb".
type URLOpener struct {
	// CertSource specifies how the opener will obtain authentication information.
	// CertSource must not be nil.
	CertSource proxy.CertSource

	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// OpenPostgresURL opens a new GCP database connection wrapped with OpenCensus instrumentation.
func (uo *URLOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	if uo.CertSource == nil {
		return nil, fmt.Errorf("gcppostgres: URLOpener CertSource is nil")
	}
	instance, dbName, err := instanceFromURL(u)
	if err != nil {
		return nil, fmt.Errorf("gcppostgres: open %v: %v", u, err)
	}

	query := u.Query()
	for k := range query {
		// Only permit parameters that do not conflict with other behavior.
		if k == "sslmode" || k == "sslcert" || k == "sslkey" || k == "sslrootcert" {
			return nil, fmt.Errorf("gcppostgres: open: extra parameter %s not allowed", k)
		}
	}
	query.Set("sslmode", "disable")

	u2 := new(url.URL)
	*u2 = *u
	u2.Scheme = "postgres"
	u2.Host = "cloudsql"
	u2.Path = "/" + dbName
	u2.RawQuery = query.Encode()
	db := sql.OpenDB(connector{
		client: &proxy.Client{
			Port:  3307,
			Certs: uo.CertSource,
		},
		instance:  instance,
		pqConn:    u2.String(),
		traceOpts: append([]ocsql.TraceOption(nil), uo.TraceOpts...),
	})
	return db, nil
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

type pqDriver struct {
	client    *proxy.Client
	instance  string
	traceOpts []ocsql.TraceOption
}

func (d pqDriver) Open(name string) (driver.Conn, error) {
	c, _ := d.OpenConnector(name)
	return c.Connect(context.Background())
}

func (d pqDriver) OpenConnector(name string) (driver.Connector, error) {
	return connector{d.client, d.instance, name, d.traceOpts}, nil
}

type connector struct {
	client    *proxy.Client
	instance  string
	pqConn    string
	traceOpts []ocsql.TraceOption
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	conn, err := pq.DialOpen(dialer{c.client, c.instance}, c.pqConn)
	if err != nil {
		return nil, err
	}
	return ocsql.WrapConn(conn, c.traceOpts...), nil
}

func (c connector) Driver() driver.Driver {
	return pqDriver{c.client, c.instance, c.traceOpts}
}

type dialer struct {
	client   *proxy.Client
	instance string
}

func (d dialer) Dial(network, address string) (net.Conn, error) {
	return d.client.Dial(d.instance)
}

func (d dialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return nil, errors.New("gcppostgres: DialTimeout not supported")
}
