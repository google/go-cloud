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

// Package runtimeconfigurator provides a runtimevar implementation with
// variables read from GCP Cloud Runtime Configurator
// (https://cloud.google.com/deployment-manager/runtime-configurator).
// Use NewVariable to construct a *runtimevar.Variable.
//
// As
//
// runtimeconfigurator exposes the following types for As:
//  - Snapshot: *pb.Variable
//  - Error: *status.Status
package runtimeconfigurator // import "gocloud.dev/runtimevar/runtimeconfigurator"

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

const (
	// endpoint is the address of the GCP Runtime Configurator API.
	endPoint = "runtimeconfig.googleapis.com:443"
)

// Dial opens a gRPC connection to the Runtime Configurator API using
// credentials from ts. It is provided as an optional helper with useful
// defaults.
//
// The second return value is a function that should be called to clean up
// the connection opened by Dial.
func Dial(ctx context.Context, ts gcp.TokenSource) (pb.RuntimeConfigManagerClient, func(), error) {
	conn, err := grpc.DialContext(ctx, endPoint,
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: ts}),
		useragent.GRPCDialOption("runtimevar"),
	)
	if err != nil {
		return nil, nil, err
	}
	return pb.NewRuntimeConfigManagerClient(conn), func() { conn.Close() }, nil
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which Parameter Store is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// NewVariable constructs a *runtimevar.Variable backed by the variable name in
// GCP Cloud Runtime Configurator.
// Runtime Configurator returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func NewVariable(client pb.RuntimeConfigManagerClient, name ResourceName, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	return runtimevar.New(newWatcher(client, name, decoder, opts)), nil
}

func newWatcher(client pb.RuntimeConfigManagerClient, name ResourceName, decoder *runtimevar.Decoder, opts *Options) driver.Watcher {
	if opts == nil {
		opts = &Options{}
	}
	return &watcher{
		client:  client,
		wait:    driver.WaitDuration(opts.WaitDuration),
		name:    name.String(),
		decoder: decoder,
	}
}

// ResourceName identifies a configuration variable.
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

// state implements driver.State.
type state struct {
	val        interface{}
	raw        *pb.Variable
	updateTime time.Time
	rawBytes   []byte
	err        error
}

// Value implements driver.State.Value.
func (s *state) Value() (interface{}, error) {
	return s.val, s.err
}

// UpdateTime implements driver.State.UpdateTime.
func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// As implements driver.State.As.
func (s *state) As(i interface{}) bool {
	if s.raw == nil {
		return false
	}
	p, ok := i.(**pb.Variable)
	if !ok {
		return false
	}
	*p = s.raw
	return true
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
	if equivalentError(err, prev.err) {
		// Same error, return nil to indicate no change.
		return nil
	}
	return s
}

// equivalentError returns true iff err1 and err2 represent an equivalent error;
// i.e., we don't want to return it to the user as a different error.
func equivalentError(err1, err2 error) bool {
	if err1 == err2 || err1.Error() == err2.Error() {
		return true
	}
	code1, code2 := grpc.Code(err1), grpc.Code(err2)
	return code1 != codes.OK && code1 != codes.Unknown && code1 == code2
}

// watcher implements driver.Watcher for configurations provided by the Runtime Configurator
// service.
type watcher struct {
	client  pb.RuntimeConfigManagerClient
	wait    time.Duration
	name    string
	decoder *runtimevar.Decoder
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	// Get the variable from the backend.
	vpb, err := w.client.GetVariable(ctx, &pb.GetVariableRequest{Name: w.name})
	if err != nil {
		return errorState(err, prev), w.wait
	}
	updateTime, err := parseUpdateTime(vpb)
	if err != nil {
		return errorState(err, prev), w.wait
	}
	// See if it's the same raw bytes as before.
	b := bytesFromProto(vpb)
	if prev != nil && bytes.Equal(b, prev.(*state).rawBytes) {
		// No change!
		return nil, w.wait
	}

	// Decode the value.
	val, err := w.decoder.Decode(b)
	if err != nil {
		return errorState(err, prev), w.wait
	}
	return &state{val: val, raw: vpb, updateTime: updateTime, rawBytes: b}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i interface{}) bool {
	// FromError converts err to a *status.Status.
	s, _ := status.FromError(err)
	if p, ok := i.(**status.Status); ok {
		*p = s
		return true
	}
	return false
}

// IsNotExist implements driver.IsNotExist.
func (*watcher) IsNotExist(err error) bool {
	return grpc.Code(err) == codes.NotFound
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
