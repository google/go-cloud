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

//+build wireinject

package main

import (
	"context"

	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/gcp/gcpcloud"
	"github.com/google/go-cloud/server"
	"github.com/google/go-cloud/wire"
)

func initialize(ctx context.Context) (*server.Server, error) {
	wire.Build(
		appSet,
		gcpcloud.Services,
		gcp.DefaultIdentity,
	)
	return nil, nil
}
