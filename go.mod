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
	cloud.google.com/go/compute v1.20.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3
	cloud.google.com/go/firestore v1.10.0
	cloud.google.com/go/iam v1.1.0
	cloud.google.com/go/kms v1.12.0
	cloud.google.com/go/longrunning v0.5.0 // indirect
	cloud.google.com/go/monitoring v1.15.0 // indirect
	cloud.google.com/go/pubsub v1.31.0
	cloud.google.com/go/secretmanager v1.11.0
	cloud.google.com/go/storage v1.30.1
	cloud.google.com/go/trace v1.10.0 // indirect
	contrib.go.opencensus.io/exporter/aws v0.0.0-20230502192102-15967c811cec
	contrib.go.opencensus.io/exporter/stackdriver v0.13.14
	contrib.go.opencensus.io/integrations/ocsql v0.1.7
	github.com/Azure/azure-amqp-common-go/v3 v3.2.3
	github.com/Azure/azure-sdk-for-go v66.0.0+incompatible // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.6.1
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.3.0
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/azkeys v0.10.0
	github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus v1.4.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.0.0
	github.com/Azure/go-amqp v1.0.1
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/GoogleCloudPlatform/cloudsql-proxy v1.33.7
	github.com/aws/aws-sdk-go v1.44.284
	github.com/aws/aws-sdk-go-v2 v1.18.1
	github.com/aws/aws-sdk-go-v2/config v1.18.27
	github.com/aws/aws-sdk-go-v2/credentials v1.13.26
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.11.70
	github.com/aws/aws-sdk-go-v2/service/kms v1.22.2
	github.com/aws/aws-sdk-go-v2/service/s3 v1.35.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.19.10
	github.com/aws/aws-sdk-go-v2/service/sns v1.20.13
	github.com/aws/aws-sdk-go-v2/service/sqs v1.23.2
	github.com/aws/aws-sdk-go-v2/service/ssm v1.36.6
	github.com/aws/smithy-go v1.13.5
	github.com/fsnotify/fsnotify v1.6.0
	github.com/gin-gonic/gin v1.7.7 // indirect
	github.com/go-sql-driver/mysql v1.7.1
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/google/go-cmp v0.5.9
	github.com/google/go-replayers/grpcreplay v1.1.0
	github.com/google/go-replayers/httpreplay v1.2.0
	github.com/google/martian v2.1.1-0.20190517191504-25dcb96d9e51+incompatible // indirect
	github.com/google/uuid v1.3.0
	github.com/google/wire v0.5.0
	github.com/googleapis/enterprise-certificate-proxy v0.2.5 // indirect
	github.com/googleapis/gax-go/v2 v2.11.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/prometheus v0.44.0 // indirect
	go.opencensus.io v0.24.0
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.10.0
	golang.org/x/net v0.11.0
	golang.org/x/oauth2 v0.9.0
	golang.org/x/sync v0.3.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/api v0.128.0
	google.golang.org/genproto v0.0.0-20230530153820-e85fd2cbaebc
	google.golang.org/grpc v1.56.0
	google.golang.org/protobuf v1.30.0
)
