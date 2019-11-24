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
	cloud.google.com/go v0.48.0 // indirect
	cloud.google.com/go/pubsub v1.0.1
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/golang/groupcache v0.0.0-20191027212112-611e8accdfc9 // indirect
	github.com/google/go-cmp v0.3.1
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/wire v0.3.0
	go.opencensus.io v0.22.2
	gocloud.dev v0.18.0
	golang.org/x/net v0.0.0-20191112182307-2180aed22343 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/sys v0.0.0-20191112214154-59a1497f0cea
	golang.org/x/xerrors v0.0.0-20191011141410-1b5146add898 // indirect
	google.golang.org/api v0.13.0
	google.golang.org/appengine v1.6.5
	google.golang.org/grpc v1.25.1 // indirect
)

replace gocloud.dev => ../../
