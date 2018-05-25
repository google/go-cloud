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

// Package xrayserver provides the diagnostic hooks for a server using
// AWS X-Ray.
package xrayserver

import (
	"github.com/google/go-cloud/goose"
	"github.com/google/go-cloud/requestlog"
	"github.com/google/go-cloud/server"

	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/xray"
	"github.com/aws/aws-sdk-go/service/xray/xrayiface"
	exporter "github.com/census-ecosystem/opencensus-go-exporter-aws"
	"go.opencensus.io/trace"
)

// Set is a Goose provider set that provides the diagnostic hooks for
// *server.Server. This set includes ServiceSet.
var Set = goose.NewSet(
	server.Set,
	ServiceSet,
	NewExporter,
	goose.Bind((*trace.Exporter)(nil), (*exporter.Exporter)(nil)),
	NopLogger{},
	goose.Bind((*requestlog.Logger)(nil), NopLogger{}),
)

// ServiceSet is a Goose provider set that provides the AWS X-Ray service
// client given an AWS session.
var ServiceSet = goose.NewSet(
	NewXRayClient,
	goose.Bind((*xrayiface.XRayAPI)(nil), (*xray.XRay)(nil)),
)

// NewExporter returns a new X-Ray exporter.
//
// The second return value is a Goose cleanup function that calls Close
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

// TODO(light): Find a real logger implementation.

// NopLogger is a requestlog.Logger that does nothing.
type NopLogger struct{}

// Log does nothing.
func (NopLogger) Log(*requestlog.Entry) {}
