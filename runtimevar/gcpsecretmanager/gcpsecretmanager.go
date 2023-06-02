// Copyright 2020 The Go Cloud Development Kit Authors
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

// Package gcpsecretmanager provides a runtimevar implementation with
// secrets read from GCP Secret Manager
// (https://cloud.google.com/secret-manager).
// Use OpenVariable to construct a *runtimevar.Variable.
//
// # URLs
//
// For runtimevar.OpenVariable, gcpsecretmanager registers for the scheme
// "gcpsecretmanager".
// The default URL opener will creating a connection using use default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// gcpsecretmanager exposes the following types for As:
//   - Snapshot: *secretmanagerpb.AccessSecretVersionResponse
//   - Error: *status.Status
package gcpsecretmanager // import "gocloud.dev/runtimevar/gcpsecretmanager"

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/google/wire"
	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

// Dial opens a gRPC connection to the Secret Manager API using
// credentials from ts. It is provided as an optional helper with useful
// defaults.
//
// The second return value is a function that should be called to clean up
// the connection opened by Dial.
func Dial(ctx context.Context, ts gcp.TokenSource) (*secretmanager.Client, func(), error) {
	client, err := secretmanager.NewClient(ctx,
		option.WithGRPCDialOption(
			grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		),
		option.WithTokenSource(oauth.TokenSource{TokenSource: ts}),
		option.WithUserAgent("runtimevar"),
	)
	if err != nil {
		return nil, nil, err
	}

	return client, func() { _ = client.Close() }, nil
}

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, new(lazyCredsOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	wire.Struct(new(URLOpener), "Client"),
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

// Scheme is the URL scheme gcpsecretmanager registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "gcpsecretmanager"

// URLOpener opens gcpsecretmanager URLs like "gcpsecretmanager://projects/[project_id]/secrets/[secret_id]".
//
// The URL Host+Path are used as the GCP Secret Manager secret key;
// see https://cloud.google.com/secret-manager
// for more details.
//
// The following query parameters are supported:
//
//   - decoder: The decoder to use. Defaults to URLOpener.Decoder, or
//     runtimevar.BytesDecoder if URLOpener.Decoder is nil.
//     See runtimevar.DecoderByName for supported values.
//   - wait: The poll interval, in time.ParseDuration formats.
//     Defaults to 30s.
type URLOpener struct {
	// Client must be set to a non-nil client authenticated with
	// Secret Manager scope or equivalent.
	Client *secretmanager.Client

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the options to pass to New.
	Options Options
}

// OpenVariableURL opens a gcpsecretmanager Secret.
func (o *URLOpener) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	q := u.Query()

	decoderName := q.Get("decoder")
	q.Del("decoder")
	decoder, err := runtimevar.DecoderByName(ctx, decoderName, o.Decoder)
	if err != nil {
		return nil, fmt.Errorf("open variable %v: invalid decoder: %v", u, err)
	}
	opts := o.Options
	if s := q.Get("wait"); s != "" {
		q.Del("wait")
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("open variable %v: invalid wait %q: %v", u, s, err)
		}
		opts.WaitDuration = d
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}
	return OpenVariable(o.Client, path.Join(u.Host, u.Path), decoder, &opts)
}

// Options sets options.
type Options struct {
	// WaitDuration controls the rate at which Secret Manager is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration
}

// OpenVariable constructs a *runtimevar.Variable backed by secretKey in GCP Secret Manager.
//
// A secretKey will look like:
//
//	projects/[project_id]/secrets/[secret_id]
//
// A project ID is a unique, user-assigned ID of the Project.
// It must be 6 to 30 lowercase letters, digits, or hyphens.
// It must start with a letter. Trailing hyphens are prohibited.
//
// A secret ID is a string with a maximum length of 255 characters and can
// contain uppercase and lowercase letters, numerals, and the hyphen (`-`) and
// underscore (`_`) characters.
//
// gcpsecretmanager package will always use the latest secret value,
// so `/version/latest` postfix must NOT be added to the secret key.
//
// You can use the full string (e.g., copied from the GCP Console), or
// construct one from its parts using SecretKey.
//
// See https://cloud.google.com/secret-manager for more details.
//
// Secret Manager returns raw bytes; provide a decoder to decode the raw bytes
// into the appropriate type for runtimevar.Snapshot.Value.
// See the runtimevar package documentation for examples of decoders.
func OpenVariable(client *secretmanager.Client, secretKey string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	w, err := newWatcher(client, secretKey, decoder, opts)
	if err != nil {
		return nil, err
	}
	return runtimevar.New(w), nil
}

var secretKeyRE = regexp.MustCompile(`^projects/[a-z][a-z0-9_\-]{4,28}[a-z0-9_]/secrets/[a-zA-Z0-9_\-]{1,255}$`)

const latestVersion = "/versions/latest"

func newWatcher(client *secretmanager.Client, secretKey string, decoder *runtimevar.Decoder, opts *Options) (driver.Watcher, error) {
	if opts == nil {
		opts = &Options{}
	}

	if !secretKeyRE.MatchString(secretKey) {
		return nil, fmt.Errorf("invalid secretKey %q; must match %v", secretKey, secretKeyRE)
	}

	return &watcher{
		client:  client,
		wait:    driver.WaitDuration(opts.WaitDuration),
		name:    secretKey,
		decoder: decoder,
	}, nil
}

// SecretKey constructs a GCP Secret Manager secret key from component parts.
// See https://cloud.google.com/secret-manager for more details.
func SecretKey(projectID gcp.ProjectID, secretID string) string {
	return "projects/" + string(projectID) + "/secrets/" + secretID
}

// state implements driver.State.
type state struct {
	val        interface{}
	raw        *secretmanagerpb.AccessSecretVersionResponse
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
	p, ok := i.(**secretmanagerpb.AccessSecretVersionResponse)
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
	code1, code2 := status.Code(err1), status.Code(err2)
	return code1 != codes.OK && code1 != codes.Unknown && code1 == code2
}

// watcher implements driver.Watcher for secrets provided by the Secret Manager service.
type watcher struct {
	client  *secretmanager.Client
	wait    time.Duration
	name    string
	decoder *runtimevar.Decoder
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	latest := w.name + latestVersion

	secret, err := w.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: latest})
	if err != nil {
		return errorState(err, prev), w.wait
	}

	if secret == nil || secret.Payload == nil || secret.Payload.Data == nil {
		return errorState(errors.New("invalid secret payload"), prev), w.wait
	}

	meta, err := w.client.GetSecretVersion(ctx, &secretmanagerpb.GetSecretVersionRequest{Name: latest})
	if err != nil {
		return errorState(err, prev), w.wait
	}

	createTime := meta.CreateTime.AsTime()

	// See if it's the same raw bytes as before.
	if prev != nil {
		prevState, ok := prev.(*state)
		if ok && prevState != nil && bytes.Equal(secret.Payload.Data, prevState.rawBytes) {
			// No change!
			return nil, w.wait
		}
	}

	// Decode the value.
	val, err := w.decoder.Decode(ctx, secret.Payload.Data)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	// A secret version is immutable.
	// The latest secret value creation time is the last time the secret value has been changed.
	// Hence set updateTime as createTime.
	return &state{val: val, raw: secret, updateTime: createTime, rawBytes: secret.Payload.Data}, w.wait
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
