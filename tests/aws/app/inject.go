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

	"github.com/google/go-cloud/aws/awscloud"
	"github.com/google/go-cloud/server"
	"github.com/google/go-cloud/wire"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

func initialize(ctx context.Context, cfg *appConfig) (*server.Server, func(), error) {
	wire.Build(appSet, awsSession, awscloud.Services)
	return nil, nil, nil
}

var awsSession = wire.NewSet(
	session.NewSessionWithOptions,
	awsOptions,
	wire.Bind((*client.ConfigProvider)(nil), (*session.Session)(nil)),
	configConfidentials,
)

func awsOptions(cfg *appConfig) session.Options {
	return session.Options{
		Config: aws.Config{
			Region: aws.String(cfg.region),
		},
	}
}

func configConfidentials(cfg *aws.Config) *credentials.Credentials {
	return cfg.Credentials
}
