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

module github.com/google/go-cloud

replace (
	cloud.google.com/go v0.23.0 => cloud.google.com/go v0.0.0-20180608230132-da2b80561ef3
)

require (
	contrib.go.opencensus.io/exporter/stackdriver v0.0.0-20180421005815-665cf5131b71
	github.com/GoogleCloudPlatform/cloudsql-proxy v0.0.0-20180321230639-1e456b1c68cb
	github.com/aws/aws-sdk-go v1.13.20
	github.com/census-ecosystem/opencensus-go-exporter-aws v0.0.0-20180411051634-41633bc1ff6b
	github.com/dnaeon/go-vcr v0.0.0-20180504081357-f8a7e8b9c630
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-sql-driver/mysql v0.0.0-20180308100310-1a676ac6e4dc
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/go-cmp v0.2.0
	github.com/gorilla/mux v1.6.1
	github.com/jtolds/gls v0.0.0-20170503224851-77f18212c9c7
	github.com/smartystreets/assertions v0.0.0-20180301161246-7678a5452ebe
	github.com/smartystreets/goconvey v0.0.0-20180222194500-ef6db91d284a
	github.com/smartystreets/gunit v0.0.0-20180314194857-6f0d6275bdcd
	github.com/stretchr/testify v1.2.1
	go.opencensus.io v0.12.0
	golang.org/x/oauth2 v0.0.0-20180603041954-1e0a3fa8ba9a
	golang.org/x/sys v0.0.0-20180329131831-378d26f46672
	golang.org/x/tools v0.0.0-20180314180217-d853e8088c62
	google.golang.org/api v0.0.0-20180606215403-8e9de5a6de6d
)
