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

// Package aws provides fundamental Goose providers for Amazon Web Services (AWS).
package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/go-cloud/goose"
)

// Set is a Goose provider set that provides a variety of common types
// from the AWS session.
var Set = goose.NewSet(
	SessionConfig,
	ConfigCredentials,
	goose.Bind((*client.ConfigProvider)(nil), (*session.Session)(nil)),
)

// DefaultSession is a Goose provider set that provides a *session.Session using
// the default options. It includes all the providers in aws.Set.
var DefaultSession = goose.NewSet(
	Set,
	session.NewSessionWithOptions,
	goose.Value(session.Options{}),
)

// SessionConfig returns sess.Config.
func SessionConfig(sess *session.Session) *aws.Config {
	return sess.Config
}

// ConfigCredentials returns cfg.Credentials.
func ConfigCredentials(cfg *aws.Config) *credentials.Credentials {
	return cfg.Credentials
}
