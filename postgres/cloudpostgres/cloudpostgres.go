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

// Package cloudpostgres provides connections to managed PostgreSQL Cloud SQL instances.
// See https://cloud.google.com/sql/docs/postgres/ for more information.

// URLs
//
// For postgres.Open, gcpostgres registers for the scheme "cloudpostgres".
// The default URL opener will creating a connection using use default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://godoc.org/gocloud.dev#hdr-URLs for background information.
//
// Escaping
//
// Go CDK supports all UTF-8 strings; to make this work with providers lacking
// full UTF-8 support, strings must be escaped (during writes) and unescaped
// (during reads). The following escapes are performed for gcsblob:
//  - Blob keys: ASCII characters 10 and 13 are escaped to "__0x<hex>__".
//    Additionally, the "/" in "../" is escaped in the same way.
//
// As
//
// gcpostgres exposes the following types for As:
//  - DB: *sql.DB
//  - Error: *googleapi.Error
package cloudpostgres // import "gocloud.dev/postgres/cloudpostgres"

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"contrib.go.opencensus.io/integrations/ocsql"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/lib/pq"
	"gocloud.dev/postgres"
)

// Params specifies how to connect to a Cloud SQL database.
type Params struct {
	// ProjectID is the project the instance is located in.
	ProjectID string
	// Region is the region the instance is located in.
	Region string
	// Instance is the name of the instance.
	Instance string

	// User is the database user to connect as.
	User string
	// Password is the database user password to use.
	Password string
	// Database is the PostgreSQL database name to connect to.
	Database string

	// Values sets additional parameters, as documented in
	// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS.
	Values url.Values

	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// URLOpener opens GCP PostgreSQL URLs
// like "cloudpostgres://user:password@myproject/us-central1/instanceId/mydb".
type URLOpener struct {
	// TraceOpts contains options for OpenCensus.
	TraceOpts  []ocsql.TraceOption
	CertSource proxy.CertSource
}

// Scheme is the URL scheme cloudpostgres registers its URLOpener under on
// postgres.DefaultMux.
const Scheme = "cloudpostgres"

func init() {
	postgres.DefaultURLMux().RegisterPostgres(Scheme, &URLOpener{})
}

// OpenPostgresURL opens a new GCP database connection wrapped with OpenCensus instrumentation.
func (uo *URLOpener) OpenPostgresURL(ctx context.Context, u *url.URL) (*sql.DB, error) {
	gcpInstance, err := GetGCPInstanceFromUrl(u)
	if err != nil {
		return nil, fmt.Errorf("cloudpostgres: open url: %s", err.Error())
	}
	projectId, region, instanceId, dbName := gcpInstance[0], gcpInstance[1], gcpInstance[2], gcpInstance[3]
	password, _ := u.User.Password()
	params := &Params{
		User:      u.User.Username(),
		ProjectID: projectId,
		Region:    region,
		Instance:  instanceId,
		Password:  password,
		Database:  dbName,
		Values:    u.Query(),
		TraceOpts: uo.TraceOpts,
	}
	return Open(ctx, uo.CertSource, params)
}

// Open opens a Cloud SQL database.
func Open(ctx context.Context, certSource proxy.CertSource, params *Params) (*sql.DB, error) {
	vals := make(url.Values)
	for k, v := range params.Values {
		// Only permit parameters that do not conflict with other behavior.
		if k == "user" || k == "password" || k == "dbname" || k == "host" || k == "port" || k == "sslmode" || k == "sslcert" || k == "sslkey" || k == "sslrootcert" {
			return nil, fmt.Errorf("cloudpostgres: open: extra parameter %s not allowed; use Params fields instead", k)
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
		Host:     "cloudsql",
		Path:     "/" + params.Database,
		RawQuery: vals.Encode(),
	}
	return sql.OpenDB(connector{
		client: &proxy.Client{
			Port:  3307,
			Certs: certSource,
		},
		instance:  params.ProjectID + ":" + params.Region + ":" + params.Instance,
		pqConn:    u.String(),
		traceOpts: append([]ocsql.TraceOption(nil), params.TraceOpts...),
	}), nil
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
	return nil, errors.New("cloudpostgres: DialTimeout not supported")
}

func GetGCPInstanceFromUrl(u *url.URL) ([]string, error) {
	gcpInstance := strings.Split(u.Host+u.Path, "/")
	if len(gcpInstance) != 4 {
		return nil, fmt.Errorf("cloudpostgres: open url: %s is not in the form project/region/instance/dbname", u.Host+u.Path)
	}

	return gcpInstance, nil
}
