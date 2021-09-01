// Copyright 2018-2019 The Go Cloud Development Kit Authors
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

module gocloud.dev

go 1.12

require (
	cloud.google.com/go v0.94.0
	cloud.google.com/go/firestore v1.5.0
	cloud.google.com/go/kms v0.1.0
	cloud.google.com/go/monitoring v0.1.0 // indirect
	cloud.google.com/go/pubsub v1.16.0
	cloud.google.com/go/secretmanager v0.1.0
	cloud.google.com/go/storage v1.16.1
	cloud.google.com/go/trace v0.1.0 // indirect
	contrib.go.opencensus.io/exporter/aws v0.0.0-20200617204711-c478e41e60e9
	contrib.go.opencensus.io/exporter/stackdriver v0.13.8
	contrib.go.opencensus.io/integrations/ocsql v0.1.7
	github.com/Azure/azure-amqp-common-go/v3 v3.1.1
	github.com/Azure/azure-pipeline-go v0.2.3
	github.com/Azure/azure-sdk-for-go v57.0.0+incompatible
	github.com/Azure/azure-service-bus-go v0.10.16
	github.com/Azure/azure-storage-blob-go v0.14.0
	github.com/Azure/go-amqp v0.13.12
	github.com/Azure/go-autorest/autorest v0.11.20
	github.com/Azure/go-autorest/autorest/adal v0.9.15
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.8
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.3 // indirect
	github.com/GoogleCloudPlatform/cloudsql-proxy v1.24.0
	github.com/aws/aws-sdk-go v1.40.34
	github.com/aws/aws-sdk-go-v2 v1.9.0
	github.com/aws/aws-sdk-go-v2/config v1.7.0
	github.com/aws/aws-sdk-go-v2/credentials v1.4.0
	github.com/aws/aws-sdk-go-v2/service/kms v1.5.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.6.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.10.0
	github.com/aws/smithy-go v1.8.0
	github.com/fsnotify/fsnotify v1.5.1
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/go-replayers/grpcreplay v1.1.0
	github.com/google/go-replayers/httpreplay v1.0.0
	github.com/google/martian v2.1.1-0.20190517191504-25dcb96d9e51+incompatible // indirect
	github.com/google/uuid v1.3.0
	github.com/google/wire v0.5.0
	github.com/googleapis/gax-go/v2 v2.1.0
	github.com/klauspost/compress v1.13.5 // indirect
	github.com/lib/pq v1.10.2
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	go.opencensus.io v0.23.0
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.0 // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5
	golang.org/x/mod v0.5.0 // indirect
	golang.org/x/net v0.0.0-20210825183410-e898025ed96a
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210831042530-f4d43177bf5e // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/api v0.56.0
	google.golang.org/genproto v0.0.0-20210831024726-fe130286e0e2
	google.golang.org/grpc v1.40.0
	nhooyr.io/websocket v1.8.7 // indirect
)
