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

// Package gcpruntimeconfig provides a runtimevar implementation with
// variables read from GCP Cloud Runtime Configurator
// (https://cloud.google.com/deployment-manager/runtime-configurator).
// Use OpenVariable to construct a *runtimevar.Variable.
//
// URLs
//
// For runtimevar.OpenVariable, gcpruntimeconfig registers for the scheme
// "gcpruntimeconfig".
// The default URL opener will creating a connection using use default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// gcpruntimeconfig exposes the following types for As:
//  - Snapshot: *pb.Variable
//  - Error: *status.Status
package gcpruntimeconfig // import "gocloud.dev/runtimevar/gcpruntimeconfig"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/wire"
	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/gcerr"
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

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, new(lazyCredsOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	URLOpener{},
)

// lazyCredsOpener obtains Application Default Credentials on the first call
// to OpenVariableURL.
type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	o.init.Do(func() {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			o.err = err
			return
		}
		client, _, err := Dial(ctx, creds.TokenSource)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, o.err)
	}
	return o.opener.OpenVariableURL(ctx, u)
}

// Scheme is the URL scheme gcpruntimeconfig registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "gcpruntimeconfig"

// URLOpener opens gcpruntimeconfig URLs like "gcpruntimeconfig://myproject/mycfg/myvar".
//
//   - The URL's host holds the GCP projectID.
//   - The first element of the URL's path holds the GCP RuntimeConfigurator ConfigID.
//   - The second element of the URL's path holds the GCP RuntimeConfigurator Variable Name.
// See https://cloud.google.com/deployment-manager/runtime-configurator/
// for more details.
//
// The following query parameters are supported:
//
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//       runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//       See runtimevar.DecoderByName for supported values.
type URLOpener struct {
	// Client must be set to a non-nil client authenticated with
	// Cloud RuntimeConfigurator scope or equivalent.
	Client pb.RuntimeConfigManagerClient

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the options to pass to New.
	Options Options
}

// OpenVariableURL opens a gcpruntimeconfig Variable for u.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(ctx, decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}
	rn, err := newResourceNameFromURL(u)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, err)
	}
	return OpenVariable(o.Client, rn, decoder, &o.Options)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which Parameter Store is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by the variable name in
// GCP Cloud Runtime Configurator.
// Runtime Configurator returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(client pb.RuntimeConfigManagerClient, name ResourceName, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
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

func newResourceNameFromURL(u *url.URL) (ResourceName, error) {
	var rn ResourceName
	rn.ProjectID = u.Host
	// Using SplitN because the variable name can be hierarchical; we
	// take the first path element as the Config and the result for the
	// Variable.
	pathParts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)
	if len(pathParts) == 2 {
		rn.Config = pathParts[0]
		rn.Variable = pathParts[1]
	}
	if rn.ProjectID == "" {
		return ResourceName{}, errors.New("URL must have a non-empty Host (the project ID)")
	}
	if rn.Config == "" || rn.Variable == "" {
		return ResourceName{}, errors.New("URL must have a Path with at 2 non-empty elements (the key config and key name)")
	}
	return rn, nil
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
	val, err := w.decoder.Decode(ctx, b)
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

// ErrorCode implements driver.ErrorCode.
func (*watcher) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
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
