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

module gocloud.dev

require (
	cloud.google.com/go v0.34.0
	contrib.go.opencensus.io/exporter/aws v0.0.0-20180906190126-dd54a7ef511e
	contrib.go.opencensus.io/exporter/ocagent v0.4.2 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.6.0
	contrib.go.opencensus.io/integrations/ocsql v0.1.2
	github.com/Azure/azure-pipeline-go v0.1.8
	github.com/Azure/azure-sdk-for-go v24.1.0+incompatible // indirect
	github.com/Azure/azure-storage-blob-go v0.0.0-20181023070848-cf01652132cc
	github.com/Azure/go-autorest v11.3.1+incompatible // indirect
	github.com/GoogleCloudPlatform/cloudsql-proxy v0.0.0-20181009230506-ac834ce67862
	github.com/SAP/go-hdb v0.13.2 // indirect
	github.com/SermoDigital/jose v0.9.2-0.20161205224733-f6df55f235c2 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190117083010-5898f5c3e520 // indirect
	github.com/araddon/gou v0.0.0-20190110011759-c797efecbb61 // indirect
	github.com/aws/aws-sdk-go v1.15.64
	github.com/boombuler/barcode v1.0.0 // indirect
	github.com/briankassouf/jose v0.9.1 // indirect
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/centrify/cloud-golang-sdk v0.0.0-20180119173102-7c97cc6fde16 // indirect
	github.com/chrismalek/oktasdk-go v0.0.0-20181212195951-3430665dfaa0 // indirect
	github.com/containerd/continuity v0.0.0-20181203112020-004b46473808 // indirect
	github.com/coreos/etcd v3.3.10+incompatible
	github.com/coreos/go-oidc v2.0.0+incompatible // indirect
	github.com/coreos/go-semver v0.2.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20181012123002-c6f51f82210d // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/dancannon/gorethink v4.0.0+incompatible // indirect
	github.com/denisenkom/go-mssqldb v0.0.0-20190111225525-2fea367d496d // indirect
	github.com/dimchansky/utfbom v1.1.0 // indirect
	github.com/dnaeon/go-vcr v1.0.1
	github.com/duosecurity/duo_api_golang v0.0.0-20190107154727-539434bf0d45 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/fullsailor/pkcs7 v0.0.0-20180613152042-8306686428a5 // indirect
	github.com/gammazero/deque v0.0.0-20180920172122-f6adf94963e4 // indirect
	github.com/gammazero/workerpool v0.0.0-20181230203049-86a96b5d5d92 // indirect
	github.com/garyburd/redigo v1.6.0 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-ldap/ldap v3.0.0+incompatible // indirect
	github.com/go-sql-driver/mysql v1.4.0
	github.com/go-stomp/stomp v2.0.2+incompatible // indirect
	github.com/gocql/gocql v0.0.0-20181124151448-70385f88b28b // indirect
	github.com/golang/groupcache v0.0.0-20180924190550-6f2cf27854a4 // indirect
	github.com/golang/protobuf v1.2.0
	github.com/google/go-cmp v0.2.0
	github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf // indirect
	github.com/google/martian v2.1.0+incompatible // indirect
	github.com/google/subcommands v0.0.0-20181012225330-46f0354f6315
	github.com/google/uuid v1.1.0
	github.com/google/wire v0.2.0
	github.com/googleapis/gax-go v2.0.0+incompatible
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75 // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.6.2
	github.com/gorilla/websocket v1.4.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.5.1 // indirect
	github.com/hashicorp/go-gcp-common v0.0.0-20180425173946-763e39302965 // indirect
	github.com/hashicorp/go-hclog v0.0.0-20190109152822-4783caec6f2e // indirect
	github.com/hashicorp/go-plugin v0.0.0-20181212150838-f444068e8f5a // indirect
	github.com/hashicorp/go-retryablehttp v0.5.1 // indirect
	github.com/hashicorp/go-sockaddr v0.0.0-20190103214136-e92cdb5343bb // indirect
	github.com/hashicorp/go-version v1.1.0 // indirect
	github.com/hashicorp/nomad v0.8.7 // indirect
	github.com/hashicorp/vault v1.0.2
	github.com/hashicorp/vault-plugin-auth-alicloud v0.0.0-20181109180636-f278a59ca3e8 // indirect
	github.com/hashicorp/vault-plugin-auth-azure v0.0.0-20181207232528-4c0b46069a22 // indirect
	github.com/hashicorp/vault-plugin-auth-centrify v0.0.0-20180816201131-66b0a34a58bf // indirect
	github.com/hashicorp/vault-plugin-auth-gcp v0.0.0-20181210200133-4d63bbfe6fcf // indirect
	github.com/hashicorp/vault-plugin-auth-jwt v0.0.0-20181031195942-f428c7791733 // indirect
	github.com/hashicorp/vault-plugin-auth-kubernetes v0.0.0-20181130162533-091d9e5d5fab // indirect
	github.com/hashicorp/vault-plugin-secrets-ad v0.0.0-20181109182834-540c0b6f1f11 // indirect
	github.com/hashicorp/vault-plugin-secrets-alicloud v0.0.0-20181109181453-2aee79cc5cbf // indirect
	github.com/hashicorp/vault-plugin-secrets-azure v0.0.0-20181207232500-0087bdef705a // indirect
	github.com/hashicorp/vault-plugin-secrets-gcp v0.0.0-20180921173200-d6445459e80c // indirect
	github.com/hashicorp/vault-plugin-secrets-gcpkms v0.0.0-20190116164938-d6b25b0b4a39 // indirect
	github.com/hashicorp/vault-plugin-secrets-kv v0.0.0-20190115203747-edbfe287c5d9 // indirect
	github.com/influxdata/influxdb v1.7.3 // indirect
	github.com/influxdata/platform v0.0.0-20190117200541-d500d3cf5589 // indirect
	github.com/jeffchao/backoff v0.0.0-20140404060208-9d7fd7aa17f2 // indirect
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/json-iterator/go v1.1.5 // indirect
	github.com/keybase/go-crypto v0.0.0-20181127160227-255a5089e85a // indirect
	github.com/lib/pq v1.0.0
	github.com/mattbaird/elastigo v0.0.0-20170123220020-2fe47fd29e4b // indirect
	github.com/michaelklishin/rabbit-hole v1.4.0 // indirect
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/ory-am/common v0.4.0 // indirect
	github.com/ory/dockertest v3.3.4+incompatible // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pquerna/cachecontrol v0.0.0-20180517163645-1555304b9b35 // indirect
	github.com/pquerna/otp v1.1.0 // indirect
	github.com/samuel/go-zookeeper v0.0.0-20180130194729-c4fab1ac1bec // indirect
	github.com/soheilhy/cmux v0.1.4 // indirect
	github.com/streadway/amqp v0.0.0-20181107104731-27835f1a64e9
	github.com/tmc/grpc-websocket-proxy v0.0.0-20171017195756-830351dc03c6 // indirect
	github.com/ugorji/go/codec v0.0.0-20181012064053-8333dd449516 // indirect
	github.com/xiang90/probing v0.0.0-20160813154853-07dd2e8dfe18 // indirect
	go.opencensus.io v0.18.1-0.20181204023538-aab39bd6a98b
	golang.org/x/crypto v0.0.0-20181203042331-505ab145d0a9
	golang.org/x/net v0.0.0-20181023162649-9b4f9f5ad519
	golang.org/x/oauth2 v0.0.0-20181017192945-9dcd33a902f4
	google.golang.org/api v0.0.0-20181021000519-a2651947f503
	google.golang.org/genproto v0.0.0-20181016170114-94acd270e44e
	google.golang.org/grpc v1.15.0
	gopkg.in/asn1-ber.v1 v1.0.0-20181015200546-f715ec2f112d // indirect
	gopkg.in/gorethink/gorethink.v4 v4.1.0 // indirect
	gopkg.in/ory-am/dockertest.v2 v2.2.3 // indirect
	gopkg.in/pipe.v2 v2.0.0-20140414041502-3c2ca4d52544
	gopkg.in/square/go-jose.v2 v2.2.2 // indirect
	k8s.io/api v0.0.0-20181221193117-173ce66c1e39 // indirect
	k8s.io/apimachinery v0.0.0-20190111195121-fa6ddc151d63 // indirect
	k8s.io/klog v0.1.0 // indirect
	layeh.com/radius v0.0.0-20190109000448-e6d9fd7a048a // indirect
	sigs.k8s.io/yaml v1.1.0 // indirect
)
