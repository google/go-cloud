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

// Package runtimeconfigurator provides a runtimevar.Driver implementation
// that reads variables from GCP Cloud Runtime Configurator.
//
// Construct a Client, then use NewVariable to construct any number of
// runtimevar.Variable objects.
package runtimeconfigurator

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cloud/wire"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
	// defaultWait is the default value for WatchOptions.WaitTime.
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
func (c *Client) NewVariable(name ResourceName, decoder *runtimevar.Decoder, opts *WatchOptions) (*runtimevar.Variable, error) {
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
		client:   c.client,
		waitTime: waitTime,
		name:     name.String(),
		decoder:  decoder,
	}), nil
}

// ResourceName identifies the full configuration variable path used by the service.
type ResourceName struct {
	ProjectID string
	Config    string
	Variable  string
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
	// WaitTime controls how quickly Watch polls. Defaults to 30 seconds.
	WaitTime time.Duration
}

// state implements driver.State.
type state struct {
	val        interface{}
	updateTime time.Time
	raw        []byte
	err        error
}

func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// errorState returns a new State with err, unless prevS also represents
// the same error, in which case it returns nil.
func errorState(err error, prevS driver.State) driver.State {
	s := &state{err: err}
	if prevS == nil {
		return s
	}
	prev := prevS.(*state)
	if prev.err == nil {
		// New error.
		return s
	}
	if err == prev.err {
		return nil
	}
	code, prevCode := grpc.Code(err), grpc.Code(prev.err)
	if code != codes.OK && code == prevCode {
		return nil
	}
	return s
}

// watcher implements driver.Watcher for configurations provided by the Runtime Configurator
// service.
type watcher struct {
	client   pb.RuntimeConfigManagerClient
	waitTime time.Duration
	name     string
	decoder  *runtimevar.Decoder
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	// Get the variable from the backend.
	vpb, err := w.client.GetVariable(ctx, &pb.GetVariableRequest{Name: w.name})
	if err != nil {
		return errorState(err, prev), w.waitTime
	}
	updateTime, err := parseUpdateTime(vpb)
	if err != nil {
		return errorState(err, prev), w.waitTime
	}
	// See if it's the same raw bytes as before.
	b := bytesFromProto(vpb)
	if prev != nil && bytes.Equal(b, prev.(*state).raw) {
		// No change!
		return nil, w.waitTime
	}

	// Decode the value.
	val, err := w.decoder.Decode(b)
	if err != nil {
		return errorState(err, prev), w.waitTime
	}
	return &state{val: val, updateTime: updateTime, raw: b}, w.waitTime
}

func bytesFromProto(vpb *pb.Variable) []byte {
	// Proto may contain either bytes or text.  If it contains text content, convert that to []byte.
	if _, isBytes := vpb.GetContents().(*pb.Variable_Value); isBytes {
		return vpb.GetValue()
	}
	return []byte(vpb.GetText())
}

func parseUpdateTime(vpb *pb.Variable) (time.Time, error) {
	updateTime, err := ptypes.Timestamp(vpb.GetUpdateTime())
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"variable message for name=%q contains invalid timestamp: %v", vpb.Name, err)
	}
	return updateTime, nil
}
