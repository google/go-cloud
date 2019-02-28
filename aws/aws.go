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
	"fmt"
	"net/url"

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
	wire.Value(session.Options{}),
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

// ConfigFromURLParams returns an aws.Config initialized based on the URL
// parameters in q. It is intended to be used by URLOpeners for AWS services.
//
// It returns an error if q contains any unknown query parameters.
func ConfigFromURLParams(q url.Values) (*aws.Config, error) {
	var cfg aws.Config
	for param, values := range q {
		value := values[0]
		switch param {
		case "region":
			cfg.Region = aws.String(value)
		case "endpoint":
			cfg.Endpoint = aws.String(value)
		case "disableSSL":
			cfg.DisableSSL = aws.Bool(value == "true")
		case "s3ForcePathStyle":
			cfg.S3ForcePathStyle = aws.Bool(value == "true")
		default:
			return nil, fmt.Errorf("unknown S3 query parameter %q", param)
		}
	}
	return &cfg, nil
}
