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

require (
	cloud.google.com/go v0.37.4
	contrib.go.opencensus.io/exporter/aws v0.0.0-20181029163544-2befc13012d0
	contrib.go.opencensus.io/exporter/stackdriver v0.10.2
	contrib.go.opencensus.io/integrations/ocsql v0.1.4
	github.com/Azure/azure-amqp-common-go v1.1.4
	github.com/Azure/azure-pipeline-go v0.1.9
	github.com/Azure/azure-sdk-for-go v27.3.0+incompatible
	github.com/Azure/azure-service-bus-go v0.4.1
	github.com/Azure/azure-storage-blob-go v0.6.0
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Azure/go-autorest v11.7.1+incompatible
	github.com/GoogleCloudPlatform/cloudsql-proxy v0.0.0-20190418212003-6ac0b49e7197
	github.com/Microsoft/go-winio v0.4.12 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/SAP/go-hdb v0.14.1 // indirect
	github.com/Shopify/sarama v1.19.0
	github.com/araddon/gou v0.0.0-20190110011759-c797efecbb61 // indirect
	github.com/armon/go-metrics v0.0.0-20190423201044-2801d9688273 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/aws/aws-sdk-go v1.19.16
	github.com/bitly/go-hostpool v0.0.0-20171023180738-a3a6125de932 // indirect
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/boombuler/barcode v1.0.0 // indirect
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/chrismalek/oktasdk-go v0.0.0-20181212195951-3430665dfaa0 // indirect
	github.com/containerd/continuity v0.0.0-20190426062206-aaeac12a7ffc // indirect
	github.com/coreos/bbolt v1.3.2 // indirect
	github.com/coreos/etcd v3.3.12+incompatible // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/dancannon/gorethink v4.0.0+incompatible // indirect
	github.com/denisenkom/go-mssqldb v0.0.0-20190423183735-731ef375ac02 // indirect
	github.com/dnaeon/go-vcr v1.0.1
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/duosecurity/duo_api_golang v0.0.0-20190308151101-6c680f768e74 // indirect
	github.com/elazarl/go-bindata-assetfs v1.0.0 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa // indirect
	github.com/garyburd/redigo v1.6.0 // indirect
	github.com/go-ldap/ldap v3.0.3+incompatible // indirect
	github.com/go-sql-driver/mysql v1.4.1
	github.com/go-stomp/stomp v2.0.2+incompatible // indirect
	github.com/gocql/gocql v0.0.0-20190423091413-b99afaf3b163 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/protobuf v1.3.1
	github.com/google/go-cmp v0.3.0
	github.com/google/go-github v17.0.0+incompatible // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/subcommands v1.0.1
	github.com/google/uuid v1.1.1
	github.com/google/wire v0.2.1
	github.com/googleapis/gax-go v2.0.2+incompatible
	github.com/gorilla/mux v1.7.1
	github.com/gorilla/websocket v1.4.0 // indirect
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/hashicorp/consul/api v1.0.1 // indirect
	github.com/hashicorp/go-memdb v1.0.1 // indirect
	github.com/hashicorp/go-version v1.2.0 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20190429164650-6ca6b85ba8ec // indirect
	github.com/hashicorp/vault v1.1.2
	github.com/hashicorp/vault-plugin-auth-alicloud v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-auth-azure v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-auth-centrify v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-auth-jwt v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-auth-kubernetes v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-secrets-ad v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-secrets-alicloud v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-secrets-azure v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-secrets-gcp v0.5.2 // indirect
	github.com/hashicorp/vault-plugin-secrets-gcpkms v0.5.1 // indirect
	github.com/hashicorp/vault-plugin-secrets-kv v0.5.1 // indirect
	github.com/influxdata/influxdb v1.7.6 // indirect
	github.com/jefferai/jsonx v1.0.0 // indirect
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/keybase/go-crypto v0.0.0-20190416182011-b785b22cc757 // indirect
	github.com/lib/pq v1.1.0
	github.com/mattbaird/elastigo v0.0.0-20170123220020-2fe47fd29e4b // indirect
	github.com/michaelklishin/rabbit-hole v1.5.0 // indirect
	github.com/nats-io/gnatsd v1.4.1
	github.com/nats-io/go-nats v1.7.2
	github.com/nats-io/nkeys v0.0.2 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/ory-am/common v0.4.0 // indirect
	github.com/ory/dockertest v3.3.4+incompatible // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pquerna/otp v1.1.0 // indirect
	github.com/samuel/go-zookeeper v0.0.0-20180130194729-c4fab1ac1bec // indirect
	github.com/soheilhy/cmux v0.1.4 // indirect
	github.com/streadway/amqp v0.0.0-20190404075320-75d898a42a94
	github.com/tidwall/pretty v0.0.0-20190325153808-1166b9ac2b65 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/ugorji/go v1.1.1 // indirect
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c // indirect
	github.com/xdg/stringprep v1.0.0 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	go.etcd.io/bbolt v1.3.2 // indirect
	go.etcd.io/etcd v3.3.12+incompatible
	go.mongodb.org/mongo-driver v1.0.1
	go.opencensus.io v0.20.2
	go.uber.org/multierr v1.1.0 // indirect
	go.uber.org/zap v1.9.1 // indirect
	golang.org/x/crypto v0.0.0-20190422183909-d864b10871cd
	golang.org/x/net v0.0.0-20190424112056-4829fb13d2c6
	golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/xerrors v0.0.0-20190410155217-1f06c39b4373
	google.golang.org/api v0.3.2
	google.golang.org/genproto v0.0.0-20190418145605-e7d98fc518a7
	google.golang.org/grpc v1.20.1
	gopkg.in/gorethink/gorethink.v4 v4.1.0 // indirect
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce // indirect
	gopkg.in/ory-am/dockertest.v2 v2.2.3 // indirect
	gopkg.in/pipe.v2 v2.0.0-20140414041502-3c2ca4d52544
	gotest.tools v2.2.0+incompatible // indirect
	layeh.com/radius v0.0.0-20190322222518-890bc1058917 // indirect
	pack.ag/amqp v0.11.0
)
