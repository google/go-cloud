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
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/certs"
	"github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/proxy"
	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/wire"
	"github.com/opencensus-integrations/ocsql"

	// mysql enables use of the MySQL dialer for the Cloud SQL Proxy.
	_ "github.com/GoogleCloudPlatform/cloudsql-proxy/proxy/dialers/mysql"
)

// CertSourceSet is a Wire provider set that binds a Cloud SQL proxy
// certificate source from an GCP-authenticated HTTP client.
var CertSourceSet = wire.NewSet(
	NewCertSource,
	wire.Bind((*proxy.CertSource)(nil), (*certs.RemoteCertSource)(nil)))

// NewCertSource creates a local certificate source that uses the given
// HTTP client. The client is assumed to make authenticated requests.
func NewCertSource(c *gcp.HTTPClient) *certs.RemoteCertSource {
	return certs.NewCertSourceOpts(&c.Client, certs.RemoteOpts{})
}

// Params specifies how to connect to a Cloud SQL database.
type Params struct {
	ProjectID string
	Region    string
	Instance  string
	User      string
	Password  string // may be empty, see https://cloud.google.com/sql/docs/sql-proxy#user
	Database  string
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
	return sql.OpenDB(connector(cfg.FormatDSN())), nil
}

// OpenGAE opens a Cloud SQL database on Google App Engine (GAE) using the
// environment variables $DB_USER, $DB_DATABASE, $DB_INSTANCE, and
// $DB_PASSWORD.
func OpenGAE(ctx context.Context, certSource proxy.CertSource) (*sql.DB, error) {
	p := &Params{}
	if p.User = os.Getenv("DB_USER"); p.User == "" {
		return nil, errors.New("opening db connection on GAE: $DB_USER is undefined")
	}
	if p.Database = os.Getenv("DB_DATABASE"); p.Database == "" {
		return nil, errors.New("opening db connection on GAE: $DB_DATABASE is undefined")
	}

	var instance string
	if instance = os.Getenv("DB_INSTANCE"); instance == "" {
		return nil, errors.New("opening db connection on GAE: $DB_INSTANCE is undefined")
	}
	parts := strings.Split(instance, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("opening db connection on GAE: $DB_INSTANCE is %q, want three fields separated by ':'", instance)
	}
	p.ProjectID = parts[0]
	p.Region = parts[1]
	p.Instance = parts[2]

	if p.Password = os.Getenv("DB_PASSWORD"); p.Password == "" {
		return nil, errors.New("opening db connection on GAE: $DB_PASSWORD is undefined")
	}
	return Open(ctx, certSource, p)
}

var dialerCounter struct {
	mu sync.Mutex
	n  int
}

type connector string

func (c connector) Connect(context.Context) (driver.Conn, error) {
	dsn := string(c)
	return mysql.MySQLDriver{}.Open(dsn)
}

func (c connector) Driver() driver.Driver {
	return ocsql.Wrap(mysql.MySQLDriver{}, ocsql.WithAllTraceOptions())
}
