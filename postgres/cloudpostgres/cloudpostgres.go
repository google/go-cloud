// Copyright 2018 The Go Cloud Authors
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
package cloudpostgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/lib/pq"
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
	// May be empty, see https://cloud.google.com/sql/docs/sql-proxy#user
	Password string
	// Database is the PostgreSQL database name to connect to.
	Database string

	// Values sets additional parameters, as documented in
	// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS.
	Values url.Values
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
		instance: params.ProjectID + ":" + params.Region + ":" + params.Instance,
		pqConn:   u.String(),
	}), nil
}

type pqDriver struct {
	client   *proxy.Client
	instance string
}

func (d pqDriver) Open(name string) (driver.Conn, error) {
	return connector{d.client, d.instance, name}.Connect(context.Background())
}

func (d pqDriver) OpenConnector(name string) (driver.Connector, error) {
	return connector{d.client, d.instance, name}, nil
}

type connector struct {
	client   *proxy.Client
	instance string
	pqConn   string
}

func (c connector) Connect(context.Context) (driver.Conn, error) {
	return pq.DialOpen(dialer{c.client, c.instance}, c.pqConn)
}

func (c connector) Driver() driver.Driver {
	return pqDriver{c.client, c.instance}
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
