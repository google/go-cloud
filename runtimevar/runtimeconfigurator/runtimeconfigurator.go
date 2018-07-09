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

// Package runtimeconfigurator provides a runtimevar driver implementation to read configurations from
// Cloud Runtime Configurator service and ability to detect changes and get updates.
//
// User constructs a Client that provides the gRPC connection, then use the client to construct any
// number of runtimevar.Variable objects using NewConfig method.
package runtimeconfigurator

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cloud/runtimevar/internal"
	"github.com/google/go-cloud/wire"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

// Set is a Wire provider set that provides *Client using a default
// connection to the Runtime Configurator API given a GCP token source.
var Set = wire.NewSet(
	Dial,
	NewClient,
)

const (
	// endpoint is the address of the GCP Runtime Configurator API.
	endPoint = "runtimeconfig.googleapis.com:443"
	// defaultWait is the default value for WatchOptions.WaitTime if not set.
	// Change the docstring for NewVariable if this time is modified.
	defaultWait = 30 * time.Second
)

// Dial opens a gRPC connection to the Runtime Configurator API.
//
// The second return value is a function that can be called to clean up
// the connection opened by Dial.
func Dial(ctx context.Context, ts gcp.TokenSource) (pb.RuntimeConfigManagerClient, func(), error) {
	conn, err := grpc.DialContext(ctx, endPoint,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: ts}),
	)
	if err != nil {
		return nil, nil, err
	}
	return pb.NewRuntimeConfigManagerClient(conn), func() { conn.Close() }, nil
}

// A Client constructs runtime variables using the Runtime Configurator API.
type Client struct {
	client pb.RuntimeConfigManagerClient
}

// NewClient returns a new client that makes calls to the given gRPC stub.
func NewClient(stub pb.RuntimeConfigManagerClient) *Client {
	return &Client{client: stub}
}

// NewVariable constructs a runtimevar.Variable object with this package as the driver
// implementation. Provide a decoder to unmarshal updated configurations into similar
// objects during the Watch call.
// If WaitTime is not set the poller will check for updates to the variable every 30 seconds.
func (c *Client) NewVariable(ctx context.Context, name ResourceName, decoder *runtimevar.Decoder, opts *WatchOptions) (*runtimevar.Variable, error) {

	if opts == nil {
		opts = &WatchOptions{}
	}
	waitTime := opts.WaitTime
	switch {
	case waitTime == 0:
		waitTime = defaultWait
	case waitTime < 0:
		return nil, fmt.Errorf("cannot have negative WaitTime option value: %v", waitTime)
	}

	return runtimevar.New(&watcher{
		client:      c.client,
		waitTime:    waitTime,
		lastRPCTime: time.Now().Add(-1 * waitTime), // Remove wait on first Watch call.
		name:        name.String(),
		decoder:     decoder,
	}), nil
}

// ResourceName identifies the full configuration variable path used by the service.
type ResourceName struct {
	ProjectID string
	Config    string
	// desc is the description of the config.
	desc     string
	Variable string
}

func (r ResourceName) configPath() string {
	return fmt.Sprintf("projects/%s/configs/%s", r.ProjectID, r.Config)
}

// String returns the full configuration variable path.
func (r ResourceName) String() string {
	return fmt.Sprintf("%s/variables/%s", r.configPath(), r.Variable)
}

// WatchOptions provide optional configurations to the Watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of RPC calls and checking for updates by the Watch method.
	// A Watcher keeps track of the last time it made an RPC, when Watch is called, it waits for
	// configured WaitTime from the last RPC before making another RPC. The smaller the value, the
	// higher the frequency of making RPCs, which also means faster rate of hitting the API quota.
	//
	// If this option is not set or set to 0, it uses defaultWait value.
	WaitTime time.Duration
}

// watcher implements driver.Watcher for configurations provided by the Runtime Configurator
// service.
type watcher struct {
	client      pb.RuntimeConfigManagerClient
	waitTime    time.Duration
	lastRPCTime time.Time
	name        string
	decoder     *runtimevar.Decoder
	bytes       []byte
	updateTime  time.Time
}

// Close implements driver.Watcher.Close.  This is a no-op for this driver.
func (w *watcher) Close() error {
	return nil
}

// WatchVariable blocks until the variable changes, the Context's Done channel closes or an error occurs. It
// implements the driver.Watcher.WatchVariable method.
func (w *watcher) WatchVariable(ctx context.Context) (driver.Variable, error) {
	return internal.Pinger(ctx, w.ping, w.waitTime)
}

func (w *watcher) ping(ctx context.Context) (*driver.Variable, error) {
	// Use GetVariables RPC and check for deltas based on the response.
	vpb, err := w.client.GetVariable(ctx, &pb.GetVariableRequest{Name: w.name})
	w.lastRPCTime = time.Now()
	if err != nil {
		return nil, err
	}
	updateTime, err := parseUpdateTime(vpb)
	if err != nil {
		return nil, err
	}

	// Determine if there are any changes based on the bytes. If there are, update cache and
	// return nil, else continue on.
	bytes := bytesFromProto(vpb)
	if !bytesNotEqual(w.bytes, bytes) {
		return nil, nil
	}
	w.bytes = bytes
	w.updateTime = updateTime
	val, err := w.decoder.Decode(bytes)
	if err != nil {
		return nil, err
	}
	return &driver.Variable{
		Value:      val,
		UpdateTime: updateTime,
	}, nil
}

func bytesFromProto(vpb *pb.Variable) []byte {
	// Proto may contain either bytes or text.  If it contains text content, convert that to []byte.
	if _, isBytes := vpb.GetContents().(*pb.Variable_Value); isBytes {
		return vpb.GetValue()
	}
	return []byte(vpb.GetText())
}

func bytesNotEqual(a []byte, b []byte) bool {
	n := len(a)
	if n != len(b) {
		return true
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

func parseUpdateTime(vpb *pb.Variable) (time.Time, error) {
	updateTime, err := ptypes.Timestamp(vpb.GetUpdateTime())
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"variable message for name=%q contains invalid timestamp: %v", vpb.Name, err)
	}
	return updateTime, nil
}
