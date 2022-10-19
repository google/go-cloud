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
	cloud.google.com/go v0.103.0 // indirect
	cloud.google.com/go/compute v1.7.0
	cloud.google.com/go/firestore v1.6.1
	cloud.google.com/go/iam v0.3.0
	cloud.google.com/go/kms v1.4.0
	cloud.google.com/go/monitoring v1.5.0 // indirect
	cloud.google.com/go/pubsub v1.24.0
	cloud.google.com/go/secretmanager v1.5.0
	cloud.google.com/go/storage v1.24.0
	cloud.google.com/go/trace v1.2.0 // indirect
	contrib.go.opencensus.io/exporter/aws v0.0.0-20200617204711-c478e41e60e9
	contrib.go.opencensus.io/exporter/stackdriver v0.13.13
	contrib.go.opencensus.io/integrations/ocsql v0.1.7
	github.com/Azure/azure-amqp-common-go/v3 v3.2.3
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.1.1
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.1.0
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/azkeys v0.8.1
	github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus v1.0.2
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v0.4.1
	github.com/Azure/go-amqp v0.17.5
	github.com/Azure/go-autorest/autorest v0.11.28
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/GoogleCloudPlatform/cloudsql-proxy v1.31.2
	github.com/aws/aws-sdk-go v1.44.68
	github.com/aws/aws-sdk-go-v2 v1.16.8
	github.com/aws/aws-sdk-go-v2/config v1.15.15
	github.com/aws/aws-sdk-go-v2/credentials v1.12.10
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.11.21
	github.com/aws/aws-sdk-go-v2/service/kms v1.18.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.27.2
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.15.14
	github.com/aws/aws-sdk-go-v2/service/sns v1.17.10
	github.com/aws/aws-sdk-go-v2/service/sqs v1.19.1
	github.com/aws/aws-sdk-go-v2/service/ssm v1.27.6
	github.com/aws/smithy-go v1.12.0
	github.com/fsnotify/fsnotify v1.5.4
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang-jwt/jwt/v4 v4.4.2 // indirect
	github.com/google/go-cmp v0.5.8
	github.com/google/go-replayers/grpcreplay v1.1.0
	github.com/google/go-replayers/httpreplay v1.1.1
	github.com/google/martian v2.1.1-0.20190517191504-25dcb96d9e51+incompatible // indirect
	github.com/google/uuid v1.3.0
	github.com/google/wire v0.5.0
	github.com/googleapis/gax-go/v2 v2.4.0
	github.com/klauspost/compress v1.15.1 // indirect
	github.com/lib/pq v1.10.6
	github.com/prometheus/prometheus v0.37.0 // indirect
	go.opencensus.io v0.23.0
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa
	golang.org/x/net v0.0.0-20220802222814-0bcc04d9c69b
	golang.org/x/oauth2 v0.0.0-20220722155238-128564f6959c
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/xerrors v0.0.0-20220609144429-65e65417b02f
	google.golang.org/api v0.91.0
	google.golang.org/genproto v0.0.0-20220802133213-ce4fa296bf78
	google.golang.org/grpc v1.48.0
	google.golang.org/protobuf v1.28.1
)
