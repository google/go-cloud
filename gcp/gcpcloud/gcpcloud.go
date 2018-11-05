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

// Package gcpcloud contains Wire providers for GCP services.
package gcpcloud

import (
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/mysql/cloudmysql"
	"github.com/google/go-cloud/runtimevar/runtimeconfigurator"
	"github.com/google/go-cloud/server/sdserver"
	"github.com/google/go-cloud/wire"
)

// GCP is a Wire provider set that includes all Google Cloud Platform services
// in this repository and authenticates using Application Default Credentials.
var GCP = wire.NewSet(Services, gcp.DefaultIdentity)

// Services is a Wire provider set that includes the default wiring for all
// Google Cloud Platform services in this repository, but does not include
// credentials. Individual services may require additional configuration.
var Services = wire.NewSet(
	cloudmysql.CertSourceSet,
	cloudmysql.Open,
	gcp.DefaultTransport,
	gcp.NewHTTPClient,
	runtimeconfigurator.Set,
	sdserver.Set)
