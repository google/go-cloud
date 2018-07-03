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

// Package awscloud contains Wire providers for AWS services.
package awscloud

import (
	"net/http"

	"github.com/google/go-cloud/aws"
	"github.com/google/go-cloud/mysql/rdsmysql"
	"github.com/google/go-cloud/runtimevar/paramstore"
	"github.com/google/go-cloud/server/xrayserver"
	"github.com/google/go-cloud/wire"
)

// AWS is a Wire provider set that includes all Amazon Web Services interface
// implementations in Go Cloud and authenticates using the default session.
var AWS = wire.NewSet(
	Services,
	aws.DefaultSession,
	wire.Value(http.DefaultClient),
)

// Services is a Wire provider set that includes the default wiring for all
// Amazon Web Services interface implementations in Go Cloud but unlike the
// AWS set, does not include credentials. Individual services may require
// additional configuration.
var Services = wire.NewSet(
	rdsmysql.Set,
	paramstore.NewClient,
	xrayserver.Set)
