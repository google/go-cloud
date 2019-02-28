// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package aws provides fundamental Wire providers for Amazon Web Services (AWS).
package aws // import "gocloud.dev/aws"

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/wire"
)

// DefaultSession is a Wire provider set that provides a *session.Session using
// the default options.
var DefaultSession = wire.NewSet(
	SessionConfig,
	ConfigCredentials,
	session.NewSessionWithOptions,
	wire.Value(session.Options{SharedConfigState: session.SharedConfigEnable}),
	wire.Bind((*client.ConfigProvider)(nil), (*session.Session)(nil)),
)

// SessionConfig returns sess.Config.
func SessionConfig(sess *session.Session) *aws.Config {
	return sess.Config
}

// ConfigCredentials returns cfg.Credentials.
func ConfigCredentials(cfg *aws.Config) *credentials.Credentials {
	return cfg.Credentials
}

// ConfigOverrider implements client.ConfigProvider by overlaying a list of
// configurations over a base configuration provider.
type ConfigOverrider struct {
	Base    client.ConfigProvider
	Configs []*aws.Config
}

// ClientConfig calls the base provider's ClientConfig method with co.Configs
// followed by the arguments given to ClientConfig.
func (co ConfigOverrider) ClientConfig(serviceName string, cfgs ...*aws.Config) client.Config {
	cfgs = append(co.Configs[:len(co.Configs):len(co.Configs)], cfgs...)
	return co.Base.ClientConfig(serviceName, cfgs...)
}
