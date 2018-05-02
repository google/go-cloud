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

// Package runtimeconfigurator provides a runtimeconfig driver implementation to read configurations from
// Cloud Runtime Configurator service and ability to detect changes and get updates.
//
// User constructs a Client that provides the gRPC connection, then use the client to construct any
// number of runtimeconfig.Config objects using NewConfig method.
package runtimeconfigurator

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cloud/runtimeconfig"
	"github.com/google/go-cloud/runtimeconfig/driver"
	"google.golang.org/api/option"
	transport "google.golang.org/api/transport/grpc"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// endpoint is the address of the GCP Runtime Configurator API.
	endPoint = "runtimeconfig.googleapis.com:443"
	// defaultWaitTimeout is the default value for WatchOptions.WaitTime if not set.
	defaultWaitTimeout = 10 * time.Minute
)

// List of authentication scopes required for using the Runtime Configurator API.
var authScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
}

// Client is a RuntimeConfigManager client.
type Client struct {
	conn *grpc.ClientConn
	// The gRPC API client.
	client pb.RuntimeConfigManagerClient
}

// NewClient constructs a Client instance from given gRPC connection.
func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	opts = append(opts, option.WithEndpoint(endPoint), option.WithScopes(authScopes...))
	conn, err := transport.Dial(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		client: pb.NewRuntimeConfigManagerClient(conn),
	}, nil
}

// Close tears down the gRPC connection used by this Client.
func (c *Client) Close() error {
	return c.conn.Close()
}

// NewConfig constructs a runtimeconfig.Config object with this package as the driver
// implementation.  Provide targetType for Config to unmarshal updated configurations into similar
// objects during the Watch call.
func (c *Client) NewConfig(ctx context.Context, name ResourceName, targetType interface{},
	opts *WatchOptions) (*runtimeconfig.Config, error) {

	if opts == nil {
		opts = &WatchOptions{}
	}
	waitTime := opts.WaitTime
	switch {
	case waitTime == 0:
		waitTime = defaultWaitTimeout
	case waitTime < 0:
		return nil, fmt.Errorf("cannot have negative WaitTime option value: %v", waitTime)
	}

	decodeFn := runtimeconfig.JSONDecode
	if opts.Decode != nil {
		decodeFn = opts.Decode
	}
	decoder := runtimeconfig.NewDecoder(targetType, decodeFn)

	return runtimeconfig.New(&watcher{
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
	Variable  string
}

// String returns the full configuration variable path.
func (r ResourceName) String() string {
	return fmt.Sprintf("projects/%s/configs/%s/variables/%s", r.ProjectID, r.Config, r.Variable)
}

// WatchOptions provide optional configurations to the Watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of making RPC and checking for updates by the Watch method.
	// A Watcher keeps track of the last time it made an RPC, when Watch is called, it waits for
	// configured WaitTime from the last RPC before making another RPC. The smaller the value, the
	// higher the frequency of making RPCs, which also means faster rate of hitting the API quota.
	//
	// If this option is not set or set to 0, it uses defaultWaitTimeout value.
	WaitTime time.Duration

	// Decode is the function to decode the configuration storage value into the specified type. If
	// this is not set, it defaults to JSON unmarshal.
	Decode runtimeconfig.Decode
}

// watcher implements driver.Watcher for configurations provided by the Runtime Configurator
// service.
type watcher struct {
	client      pb.RuntimeConfigManagerClient
	waitTime    time.Duration
	lastRPCTime time.Time
	name        string
	decoder     *runtimeconfig.Decoder
	bytes       []byte
	isDeleted   bool
	updateTime  time.Time
}

// Close implements driver.Watcher.Close.  This is a no-op for this driver.
func (w *watcher) Close() error {
	return nil
}

// Watch blocks until the file changes, the Context's Done channel closes or an error occurs. It
// implements the driver.Watcher.Watch method.
func (w *watcher) Watch(ctx context.Context) (driver.Config, error) {
	zeroConfig := driver.Config{}

	// Loop to check for changes or continue waiting.
	for {
		// Block until waitTime or context cancelled/timed out.
		t := time.NewTimer(w.waitTime - time.Now().Sub(w.lastRPCTime))
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return zeroConfig, ctx.Err()
		}

		// Use GetVariables RPC and check for deltas based on the response.
		vpb, err := w.client.GetVariable(ctx, &pb.GetVariableRequest{Name: w.name})
		w.lastRPCTime = time.Now()
		if err == nil {
			updateTime, err := parseUpdateTime(vpb)
			if err != nil {
				return zeroConfig, err
			}

			// Determine if there are any changes based on the bytes. If there are, update cache and
			// return nil, else continue on.
			bytes := bytesFromProto(vpb)
			if w.isDeleted || bytesNotEqual(w.bytes, bytes) {
				w.bytes = bytes
				w.updateTime = updateTime
				w.isDeleted = false
				val, err := w.decoder.Decode(bytes)
				if err != nil {
					return zeroConfig, err
				}
				return driver.Config{
					Value:      val,
					UpdateTime: updateTime,
				}, nil
			}

		} else {
			if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
				return zeroConfig, err
			}
			// For RPC not found error, if last known state is not deleted, mark isDeleted and
			// return error, else treat as no change has occurred.
			if !w.isDeleted {
				w.isDeleted = true
				w.updateTime = time.Now().UTC()
				return zeroConfig, err
			}
		}
	}
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
