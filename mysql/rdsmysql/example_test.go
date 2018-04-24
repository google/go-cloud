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

package rdsmysql_test

import (
	"context"
	"crypto/x509"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/go-cloud/mysql/rdsmysql"
)

func Example() {
	ctx := context.Background()
	sess := session.Must(session.NewSession())
	certs, err := rdsmysql.FetchCertificates(ctx, http.DefaultClient)
	if err != nil {
		panic(err)
	}
	certPool := x509.NewCertPool()
	for _, c := range certs {
		certPool.AddCert(c)
	}
	db, err := rdsmysql.Open(ctx, sess.Config.Credentials, certPool, &rdsmysql.Params{
		// Replace these with your actual settings.
		Endpoint: "example01.xyzzy.us-west-1.rds.amazonaws.com:3306",
		Region:   "us-west-1",
		User:     "myrole",
		Database: "test",
	})
	if err != nil {
		panic(err)
	}

	// Use database in your program.
	db.Exec("CREATE TABLE foo (bar INT);")
}
