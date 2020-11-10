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

module gocloud.dev/internal/contributebot

go 1.12

require (
	cloud.google.com/go/pubsub v1.8.2
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/google/go-cmp v0.5.2
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/wire v0.4.0
	go.opencensus.io v0.22.5
	gocloud.dev v0.20.0
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	golang.org/x/sys v0.0.0-20201110211018-35f3e6cf4a65
	google.golang.org/api v0.35.0
	google.golang.org/appengine v1.6.7
)

replace gocloud.dev => ../../
