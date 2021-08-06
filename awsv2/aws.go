// Copyright 2021 The Go Cloud Development Kit Authors
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

// Package awsv2 provides fundamental Wire providers for Amazon Web Services (AWS).
package awsv2 // import "gocloud.dev/awsv2"

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/wire"
)

// DefaultConfig is a Wire provider set that provides a aws.Configusing
// the default options.
var DefaultConfig = wire.NewSet(
	NewDefaultConfig,
)

// NewDefaultConfig returns an aws.Config using the default options.
func NewDefaultConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx)
}

// ConfigFromURLParams returns an aws.Config initialized based on the URL
// parameters in q. It is intended to be used by URLOpeners for AWS services.
// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/aws#Config
//
// It returns an error if q contains any unknown query parameters; callers
// should remove any query parameters they know about from q before calling
// ConfigFromURLParams.
//
// The following query options are supported:
//  - region: The AWS region for requests; sets WithRegion.
//  - profile: The shared config profile to use; sets SharedConfigProfile.
func ConfigFromURLParams(ctx context.Context, q url.Values) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error
	for param, values := range q {
		value := values[0]
		switch param {
		case "region":
			opts = append(opts, config.WithRegion(value))
		case "profile":
			opts = append(opts, config.WithSharedConfigProfile(value))
		default:
			return aws.Config{}, fmt.Errorf("unknown query parameter %q", param)
		}
	}
	return config.LoadDefaultConfig(ctx, opts...)
}
