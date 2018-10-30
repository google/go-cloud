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
package cloudpostgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"time"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/lib/pq"
)

// Params specifies how to connect to a Cloud SQL database.
type Params struct {
	ProjectID string
	Region    string
	Instance  string
	PQConn    string // format is documented at https://godoc.org/github.com/lib/pq#hdr-Connection_String_Parameters
}

// Open opens a Cloud SQL database.
func Open(ctx context.Context, certSource proxy.CertSource, params *Params) (*sql.DB, error) {
	return sql.OpenDB(connector{
		client: &proxy.Client{
			Port:  3307,
			Certs: certSource,
		},
		instance: params.ProjectID + ":" + params.Region + ":" + params.Instance,
		pqConn:   "sslmode=disable " + params.PQConn,
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
