// Copyright 2018 Google LLC
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

// +build gcp

package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

var notImplemented = errors.New("not implemented")

type spannerRows struct {
	iter *spanner.RowIterator

	// We need to fetch a row to find out the number columns, so all rows
	// have to be passed through these variables
	nextRow *spanner.Row
	nextErr error
}

func (r *spannerRows) Columns() []string {
	r.ensureRow()
	if r.nextRow == nil {
		return []string{}
	}
	return r.nextRow.ColumnNames()
}

func (r *spannerRows) Close() error {
	r.iter.Stop()
	return nil
}

func (r *spannerRows) ensureRow() {
	if r.nextRow == nil && r.nextErr == nil {
		r.nextRow, r.nextErr = r.iter.Next()
	}
}

func (r *spannerRows) Next(dest []driver.Value) error {
	r.ensureRow()
	_, err := r.nextRow, r.nextErr
	if err == iterator.Done {
		return io.EOF
	}
	// TODO: convert types... fun.
	panic("TODO")
	r.nextRow, r.nextErr = r.iter.Next()
	return err
}

func makeNamedValues(vals []driver.Value) []driver.NamedValue {
	nvals := make([]driver.NamedValue, len(vals))
	for i, v := range vals {
		nvals[i] = driver.NamedValue{
			Name:    "",
			Ordinal: i + 1,
			Value:   v,
		}
	}
	return nvals
}

func makeParamsMap(nvals []driver.NamedValue) map[string]interface{} {
	ret := make(map[string]interface{})
	for i, v := range nvals {
		if v.Name == "" {
			ret[fmt.Sprintf("%d", i+1)] = v.Value
		}
		ret[v.Name] = v.Value
	}
	return ret
}

type spannerStmt struct {
	query  string
	client *spanner.Client
}

func (s *spannerStmt) Close() error {
	return notImplemented
}

func (s *spannerStmt) NumInput() int {
	return -1
}

func (s *spannerStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), makeNamedValues(args))
}

func (s *spannerStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return nil, notImplemented
}

func (s *spannerStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), makeNamedValues(args))
}

func (s *spannerStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	stmt := spanner.Statement{
		SQL:    s.query,
		Params: makeParamsMap(args),
	}
	iter := s.client.Single().Query(ctx, stmt)
	return &spannerRows{iter: iter}, nil
}

// Ensure we implement driver.Stmt{Exec,Query}Context
var _ driver.StmtExecContext = (*spannerStmt)(nil)
var _ driver.StmtQueryContext = (*spannerStmt)(nil)

type spannerConn struct {
	client *spanner.Client
}

func (c *spannerConn) Prepare(query string) (driver.Stmt, error) {
	return &spannerStmt{query: query, client: c.client}, nil
}

func (c *spannerConn) Begin() (driver.Tx, error) {
	return nil, notImplemented
}

func (c *spannerConn) Close() error {
	c.client.Close()
	return nil
}

type spannerConnector struct {
	dbName string
	driver driver.Driver
}

func (c *spannerConnector) Connect(ctx context.Context) (driver.Conn, error) {
	client, err := spanner.NewClient(ctx, c.dbName)
	if err != nil {
		return nil, err
	}
	return &spannerConn{client: client}, nil
}

func (c *spannerConnector) Driver() driver.Driver {
	return c.driver
}

type spannerDriver struct {
}

func (d *spannerDriver) Open(name string) (driver.Conn, error) {
	cr, err := d.OpenConnector(name)
	if err != nil {
		return nil, err
	}
	return cr.Connect(context.Background())
}

func (d *spannerDriver) OpenConnector(name string) (driver.Connector, error) {
	return &spannerConnector{dbName: name, driver: d}, nil
}

// Ensure we implement driver.DriverContext
var _ driver.DriverContext = (*spannerDriver)(nil)

func init() {
	sql.Register("spanner", new(spannerDriver))
}
