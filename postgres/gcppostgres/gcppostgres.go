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
// # URLs
//
// For postgres.Open, gcppostgres registers for the scheme "gcppostgres".
// The default URL opener will create a connection using the default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
//
// # IP Type
//
// By default, connections use auto-IP selection (public IP if available,
// otherwise private IP), matching the behavior of the legacy cloudsql-proxy.
// To use a specific IP type, set URLOpener.IPType or pass the "ip_type" query
// parameter in the URL (requires URLOpener.Dialer to be set):
//
//	gcppostgres://user:pass@project/region/instance/dbname?ip_type=psc
//
// Valid values for ip_type: auto, public, private, psc.
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
	"slices"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/XSAM/otelsql"
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
		// Ignore cleanup: the dialer lives for the process lifetime in this global opener.
		d, _, err := cloudsql.NewDialerWithIAM(ctx, creds.TokenSource)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Dialer: d}
	})
	if o.err != nil {
		return nil, fmt.Errorf("gcppostgres open %v: %v", u, o.err)
	}
	return o.opener.OpenPostgresURL(ctx, u)
}

// URLOpener opens GCP PostgreSQL URLs
// like "gcppostgres://user:password@myproject/us-central1/instanceId/mydb".
type URLOpener struct {
	// Dialer creates Cloud SQL connections using the Cloud SQL Go Connector.
	// Supports PSC and explicit IP type selection via the "ip_type" URL query
	// parameter. If both Dialer and CertSource are set, Dialer takes precedence.
	Dialer *cloudsqlconn.Dialer

	// CertSource specifies how the opener will obtain authentication information.
	//
	// Deprecated: Use Dialer instead. CertSource does not support PSC or
	// explicit IP type selection. Ignored if Dialer is also set.
	CertSource proxy.CertSource

	// TraceOpts contains options for OpenTelemetry.
	TraceOpts []otelsql.Option
}

// OpenPostgresURL opens a new GCP database connection wrapped with OpenTelemetry instrumentation.
func (uo *URLOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	if uo.Dialer == nil && uo.CertSource == nil {
		return nil, fmt.Errorf("gcppostgres: URLOpener Dialer and CertSource are both nil")
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

	if uo.Dialer != nil {
		// New path: Cloud SQL Go Connector with IP type and PSC support.
		ipType := cloudsql.IPTypeAuto
		if s := query.Get("ip_type"); s != "" {
			parsed, err := parseIPType(s)
			if err != nil {
				return nil, fmt.Errorf("gcppostgres: open: %v", err)
			}
			ipType = parsed
			query.Del("ip_type")
		}
		u2.RawQuery = query.Encode()
		return sql.OpenDB(connector{
			dialer:    uo.Dialer,
			instance:  instance,
			dialOpts:  ipType.DialOptions(),
			pqConn:    u2.String(),
			traceOpts: append([]otelsql.Option(nil), uo.TraceOpts...),
		}), nil
	}

	// Legacy path: cloudsql-proxy v1 (no PSC or IP type selection).
	query.Del("ip_type") // strip even if set, silently ignore for legacy path
	u2.RawQuery = query.Encode()
	return sql.OpenDB(legacyConnector{
		client: &proxy.Client{
			Port:  3307,
			Certs: uo.CertSource,
		},
		instance:  instance,
		pqConn:    u2.String(),
		traceOpts: append([]otelsql.Option(nil), uo.TraceOpts...),
	}), nil
}

func instanceFromURL(u *url.URL) (instance, db string, _ error) {
	path := u.Host + u.Path // everything after scheme but before query or fragment
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 4 {
		return "", "", fmt.Errorf("%s is not in the form project/region/instance/dbname", path)
	}
	if slices.Contains(parts, "") {
		return "", "", fmt.Errorf("%s is not in the form project/region/instance/dbname", path)
	}
	return parts[0] + ":" + parts[1] + ":" + parts[2], parts[3], nil
}

func parseIPType(s string) (cloudsql.IPType, error) {
	switch strings.ToLower(s) {
	case "auto", "":
		return cloudsql.IPTypeAuto, nil
	case "public":
		return cloudsql.IPTypePublic, nil
	case "private":
		return cloudsql.IPTypePrivate, nil
	case "psc":
		return cloudsql.IPTypePSC, nil
	default:
		return 0, fmt.Errorf("unknown ip_type %q (valid: auto, public, private, psc)", s)
	}
}

// --- New path: Cloud SQL Go Connector ---

type pqDriver struct {
	dialer    *cloudsqlconn.Dialer
	instance  string
	dialOpts  []cloudsqlconn.DialOption
	traceOpts []otelsql.Option
}

func (d pqDriver) Open(name string) (driver.Conn, error) {
	c, _ := d.OpenConnector(name)
	return c.Connect(context.Background())
}

func (d pqDriver) OpenConnector(name string) (driver.Connector, error) {
	return connector{d.dialer, d.instance, d.dialOpts, name, d.traceOpts}, nil
}

type connector struct {
	dialer    *cloudsqlconn.Dialer
	instance  string
	dialOpts  []cloudsqlconn.DialOption
	pqConn    string
	traceOpts []otelsql.Option
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	conn, err := pq.DialOpen(dialer{c.dialer, c.instance, c.dialOpts}, c.pqConn)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c connector) Driver() driver.Driver {
	return otelsql.WrapDriver(pqDriver{c.dialer, c.instance, c.dialOpts, c.traceOpts}, c.traceOpts...)
}

type dialer struct {
	d        *cloudsqlconn.Dialer
	instance string
	dialOpts []cloudsqlconn.DialOption
}

func (d dialer) Dial(network, address string) (net.Conn, error) {
	return d.d.Dial(context.Background(), d.instance, d.dialOpts...)
}

func (d dialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return nil, errors.New("gcppostgres: DialTimeout not supported")
}

// --- Legacy path: cloudsql-proxy v1 ---

type legacyPqDriver struct {
	client    *proxy.Client
	instance  string
	traceOpts []otelsql.Option
}

func (d legacyPqDriver) Open(name string) (driver.Conn, error) {
	c, _ := d.OpenConnector(name)
	return c.Connect(context.Background())
}

func (d legacyPqDriver) OpenConnector(name string) (driver.Connector, error) {
	return legacyConnector{d.client, d.instance, name, d.traceOpts}, nil
}

type legacyConnector struct {
	client    *proxy.Client
	instance  string
	pqConn    string
	traceOpts []otelsql.Option
}

func (c legacyConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := pq.DialOpen(legacyDialer{c.client, c.instance}, c.pqConn)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c legacyConnector) Driver() driver.Driver {
	return otelsql.WrapDriver(legacyPqDriver{c.client, c.instance, c.traceOpts}, c.traceOpts...)
}

type legacyDialer struct {
	client   *proxy.Client
	instance string
}

func (d legacyDialer) Dial(network, address string) (net.Conn, error) {
	return d.client.Dial(d.instance)
}

func (d legacyDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	return nil, errors.New("gcppostgres: DialTimeout not supported")
}
