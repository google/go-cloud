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

//+build wireinject

package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/fileblob"
	"github.com/google/go-cloud/requestlog"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/filevar"
	"github.com/google/go-cloud/server"
	"github.com/google/go-cloud/wire"
	"go.opencensus.io/trace"
)

// Configure these as you wish.
const (
	fileBucketDir = "blobbucket"
	motdFile      = "motd.txt"
)

func setupLocal(ctx context.Context) (*app, func(), error) {
	panic(wire.Build(
		wire.Value(requestlog.Logger(nil)),
		wire.Value(trace.Exporter(nil)),
		server.Set,
		appSet,
		dialLocalSQL,
		localBucket,
		localRuntimeVar,
	))
}

func localBucket() (*blob.Bucket, error) {
	return fileblob.NewBucket("blobbucket")
}

func dialLocalSQL() (*sql.DB, error) {
	cfg := &mysql.Config{
		Net:                  "tcp",
		Addr:                 "localhost",
		DBName:               "guestbook",
		User:                 "guestbook",
		Passwd:               "xyzzy",
		AllowNativePasswords: true,
	}
	return sql.Open("mysql", cfg.FormatDSN())
}

func localRuntimeVar() (*runtimevar.Variable, func(), error) {
	v, err := filevar.NewVariable(motdFile, runtimevar.StringDecoder, &filevar.WatchOptions{
		WaitTime: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, err
	}
	return v, func() { v.Close() }, nil
}
