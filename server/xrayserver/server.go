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

// Package xrayserver provides the diagnostic hooks for a server using
// AWS X-Ray.
package xrayserver // import "gocloud.dev/server/xrayserver"

import (
	"fmt"
	"os"

	exporter "contrib.go.opencensus.io/exporter/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/xray"
	"github.com/aws/aws-sdk-go/service/xray/xrayiface"
	"github.com/google/wire"
	"go.opencensus.io/trace"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
)

// Set is a Wire provider set that provides the diagnostic hooks for
// *server.Server. This set includes ServiceSet.
var Set = wire.NewSet(
	server.Set,
	ServiceSet,
	NewExporter,
	wire.Bind(new(trace.Exporter), new(*exporter.Exporter)),
	NewRequestLogger,
	wire.Bind(new(requestlog.Logger), new(*requestlog.NCSALogger)),
)

// ServiceSet is a Wire provider set that provides the AWS X-Ray service
// client given an AWS session.
var ServiceSet = wire.NewSet(
	NewXRayClient,
	wire.Bind(new(xrayiface.XRayAPI), new(*xray.XRay)),
)

// NewExporter returns a new X-Ray exporter.
//
// The second return value is a Wire cleanup function that calls Close
// on the exporter, ignoring the error.
func NewExporter(api xrayiface.XRayAPI) (*exporter.Exporter, func(), error) {
	e, err := exporter.NewExporter(exporter.WithAPI(api))
	if err != nil {
		return nil, nil, err
	}
	return e, func() { e.Close() }, nil
}

// NewXRayClient returns a new AWS X-Ray client.
func NewXRayClient(p client.ConfigProvider) *xray.XRay {
	return xray.New(p)
}

// NewRequestLogger returns a request logger that sends entries to stdout.
func NewRequestLogger() *requestlog.NCSALogger {
	return requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Fprintln(os.Stderr, e) })
}
