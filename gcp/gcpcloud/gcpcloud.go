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

// Package gcpcloud contains Wire providers for GCP services.
package gcpcloud // import "gocloud.dev/gcp/gcpcloud"

import (
	"github.com/google/wire"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/docstore/gcpfirestore"
	"gocloud.dev/gcp"
	"gocloud.dev/gcp/cloudsql"
	"gocloud.dev/pubsub/gcppubsub"
	"gocloud.dev/runtimevar/gcpruntimeconfig"
	"gocloud.dev/secrets/gcpkms"
	"gocloud.dev/server/sdserver"
)

// GCP is a Wire provider set that includes all Google Cloud Platform services
// in this repository and authenticates using Application Default Credentials.
var GCP = wire.NewSet(Services, gcp.DefaultIdentity)

// Services is a Wire provider set that includes the default wiring for all
// Google Cloud Platform services in this repository, but does not include
// credentials. Individual services may require additional configuration.
var Services = wire.NewSet(
	gcp.DefaultTransport,
	gcp.NewHTTPClient,

	gcpruntimeconfig.Set,
	gcpkms.Set,
	gcppubsub.Set,
	gcsblob.Set,
	cloudsql.CertSourceSet,
	gcpfirestore.Set,
	sdserver.Set,
)
