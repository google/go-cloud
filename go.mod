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

go 1.25.8

require (
	cloud.google.com/go/compute/metadata v0.9.0
	cloud.google.com/go/firestore v1.24.0
	cloud.google.com/go/iam v1.12.0
	cloud.google.com/go/kms v1.33.0
	cloud.google.com/go/pubsub v1.51.0
	cloud.google.com/go/pubsub/v2 v2.6.1
	cloud.google.com/go/secretmanager v1.21.0
	cloud.google.com/go/storage v1.64.0
	github.com/Azure/azure-amqp-common-go/v3 v3.2.3
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.22.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.14.0
	github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus v1.10.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys v1.5.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.8.0
	github.com/Azure/go-amqp v1.7.0
	github.com/GoogleCloudPlatform/cloudsql-proxy v1.38.2
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.59.0
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.35.0
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator v0.59.0
	github.com/XSAM/otelsql v0.43.0
	github.com/aws/aws-sdk-go-v2 v1.43.0
	github.com/aws/aws-sdk-go-v2/config v1.32.31
	github.com/aws/aws-sdk-go-v2/credentials v1.19.30
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.20.54
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression v1.8.54
	github.com/aws/aws-sdk-go-v2/feature/rds/auth v1.6.31
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.3.5
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.62.0
	github.com/aws/aws-sdk-go-v2/service/kms v1.55.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.106.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.44.0
	github.com/aws/aws-sdk-go-v2/service/sns v1.42.0
	github.com/aws/aws-sdk-go-v2/service/sqs v1.46.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.73.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.0
	github.com/aws/smithy-go v1.27.4
	github.com/fsnotify/fsnotify v1.10.1
	github.com/go-sql-driver/mysql v1.10.0
	github.com/google/go-cmp v0.7.0
	github.com/google/go-replayers/grpcreplay v1.3.0
	github.com/google/go-replayers/httpreplay v1.2.0
	github.com/google/uuid v1.6.0
	github.com/google/wire v0.7.0
	github.com/googleapis/gax-go/v2 v2.23.0
	github.com/lib/pq v1.12.3
	go.opentelemetry.io/contrib/detectors/aws/ec2 v1.38.0
	go.opentelemetry.io/contrib/detectors/gcp v1.44.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0
	go.opentelemetry.io/contrib/propagators/aws v1.44.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/metric v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/metric v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
	golang.org/x/crypto v0.54.0
	golang.org/x/net v0.57.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sync v0.22.0
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da
	google.golang.org/api v0.290.0
	google.golang.org/genproto v0.0.0-20260724162435-b2f20204f0df
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.22.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/longrunning v1.2.0 // indirect
	cloud.google.com/go/monitoring v1.30.0 // indirect
	cloud.google.com/go/trace v1.16.0 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.7.2 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.35.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.59.0 // indirect
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.14 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.36.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.12.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.37.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/martian/v3 v3.3.3 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.19 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/spiffe/go-spiffe/v2 v2.8.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.69.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260724162435-b2f20204f0df // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260724162435-b2f20204f0df // indirect
)
