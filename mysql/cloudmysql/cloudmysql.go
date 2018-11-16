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

// Package cloudmysql provides connections to managed MySQL Cloud SQL instances.
package cloudmysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/certs"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/gcp/cloudsql"
	"github.com/opencensus-integrations/ocsql"

	// mysql enables use of the MySQL dialer for the Cloud SQL Proxy.
	_ "github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/dialers/mysql"
)

// CertSourceSet is a Wire provider set that binds a Cloud SQL proxy
// certificate source from an GCP-authenticated HTTP client.
//
// Deprecated: Use cloudsql.CertSourceSet.
var CertSourceSet = cloudsql.CertSourceSet

// NewCertSource creates a local certificate source that uses the given
// HTTP client. The client is assumed to make authenticated requests.
//
// Deprecated: Use cloudsql.NewCertSource.
func NewCertSource(c *gcp.HTTPClient) *certs.RemoteCertSource {
	return cloudsql.NewCertSource(c)
}

// Params specifies how to connect to a Cloud SQL database.
type Params struct {
	// ProjectID specifies the GCP project associated with the
	// CloudSQL instance.
	ProjectID string

	// Region is the GCP region containing the CloudSQL instance.
	Region string

	// Instance is the CloudSQL instance name. See
	// https://cloud.google.com/sql/docs/mysql/create-instance
	// for background.
	Instance string

	// User is the username used to connect to the database.
	User string

	// Password is the password used to connect to the database.
	// It may be empty, see https://cloud.google.com/sql/docs/sql-proxy#user
	Password string

	// Database is the name of the database to connect to.
	Database string

	// TraceOpts contains options for OpenCensus.
	TraceOpts []ocsql.TraceOption
}

// Open opens a Cloud SQL database.
func Open(ctx context.Context, certSource proxy.CertSource, params *Params) (*sql.DB, error) {
	// TODO(light): Avoid global registry once https://github.com/go-sql-driver/mysql/issues/771 is fixed.
	dialerCounter.mu.Lock()
	dialerNum := dialerCounter.n
	dialerCounter.mu.Unlock()
	client := &proxy.Client{
		Port:  3307,
		Certs: certSource,
	}
	dialerName := fmt.Sprintf("github.com/google/go-cloud/mysql/gcpmysql/%d", dialerNum)
	mysql.RegisterDial(dialerName, client.Dial)

	cfg := &mysql.Config{
		AllowNativePasswords: true,
		Net:                  dialerName,
		Addr:                 params.ProjectID + ":" + params.Region + ":" + params.Instance,
		User:                 params.User,
		Passwd:               params.Password,
		DBName:               params.Database,
	}
	return sql.OpenDB(connector{cfg.FormatDSN(), params.TraceOpts}), nil
}

var dialerCounter struct {
	mu sync.Mutex
	n  int
}

type connector struct {
	dsn       string
	traceOpts []ocsql.TraceOption
}

func (c connector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.Driver().Open(c.dsn)
}

func (c connector) Driver() driver.Driver {
	return ocsql.Wrap(mysql.MySQLDriver{}, c.traceOpts...)
}
