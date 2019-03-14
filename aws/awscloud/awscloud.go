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

// Package awscloud contains Wire providers for AWS services.
package awscloud // import "gocloud.dev/aws/awscloud"

import (
	"net/http"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/wire"
	"gocloud.dev/aws"
	"gocloud.dev/aws/rds"
	"gocloud.dev/server/xrayserver"
	"gocloud.dev/blob/s3blob"
	"gocloud.dev/pubsub/awspubsub"
	"gocloud.dev/secrets/awskms"
	"gocloud.dev/runtimevar/paramstore"
)

// AWS is a Wire provider set that includes all Amazon Web Services interface
// implementations in the Go CDK and authenticates using the default session.
var AWS = wire.NewSet(
	Services,
	aws.DefaultSession,
	wire.Value(http.DefaultClient),
)

// Services is a Wire provider set that includes the default wiring for all
// Amazon Web Services interface implementations in the Go CDK but unlike the
// AWS set, does not include credentials. Individual services may require
// additional configuration.
var Services = wire.NewSet(
	// gocloud.dev/blob
	s3blob.Options{},
	s3blob.URLOpener{},

	// gocloud.dev/pubsub
	awspubsub.SubscriptionOptions{},
	awspubsub.TopicOptions{},
	awspubsub.URLOpener{},
	sns.New,
	sqs.New,

	// gocloud.dev/runtimevar
	paramstore.Options{},
	paramstore.URLOpener{},

	// gocloud.dev/secrets
	awskms.KeeperOptions{},
	awskms.Dial,
	awskms.URLOpener{},

	// gocloud.dev/server
	rds.CertFetcherSet,
	xrayserver.Set,
)
