// Copyright 2019 The Go Cloud Development Kit Authors
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

// Package hashivault provides a runtimevar implementation with variables
// backed by HashiCorp Vault's KV Secrets Engine.
// Use OpenVariable to construct a *runtimevar.Variable.
//
// # URLs
//
// For runtimevar.OpenVariable, hashivault registers for the scheme "hashivault".
// The default URL opener will dial a Vault server using the environment variables
// "VAULT_SERVER_URL" (or "VAULT_ADDR") and "VAULT_SERVER_TOKEN" (or "VAULT_TOKEN").
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// hashivault exposes the following types for As:
//   - Snapshot: *api.Secret
//   - Error: *SecretError, *api.ResponseError
package hashivault // import "gocloud.dev/runtimevar/hashivault"

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
)

func init() {
	runtimevar.DefaultURLMux().RegisterVariable(Scheme, new(defaultDialer))
}

// Scheme is the URL scheme hashivault registers its URLOpener under on runtimevar.DefaultMux.
const Scheme = "hashivault"

// SecretError represents an error from a Vault operation.
type SecretError struct {
	// Code is the error code (e.g., 404 for not found).
	Code int
	// Message is the error message.
	Message string
}

func (e *SecretError) Error() string {
	return fmt.Sprintf("hashivault: %s (code %d)", e.Message, e.Code)
}

func newNotFoundError(path string) *SecretError {
	return &SecretError{
		Code:    404,
		Message: fmt.Sprintf("secret not found at path %q", path),
	}
}

func newInvalidDataError(path, reason string) *SecretError {
	return &SecretError{
		Code:    400,
		Message: fmt.Sprintf("invalid data at path %q: %s", path, reason),
	}
}

// Config is the authentication configuration for the Vault server.
type Config struct {
	// Token is the access token the Vault client uses to talk to the server.
	// See https://www.vaultproject.io/docs/concepts/tokens.html for more
	// information.
	Token string
	// APIConfig is used to configure the creation of the client.
	APIConfig api.Config
}

// Dial creates a Vault API client using the provided configuration.
func Dial(ctx context.Context, cfg *Config) (*api.Client, error) {
	if cfg == nil {
		return nil, errors.New("no auth Config provided")
	}
	c, err := api.NewClient(&cfg.APIConfig)
	if err != nil {
		return nil, err
	}
	if cfg.Token != "" {
		c.SetToken(cfg.Token)
	}
	return c, nil
}

func getVaultURL() (string, error) {
	if url := os.Getenv("VAULT_SERVER_URL"); url != "" {
		return url, nil
	}
	if url := os.Getenv("VAULT_ADDR"); url != "" {
		return url, nil
	}
	return "", errors.New("neither VAULT_SERVER_URL nor VAULT_ADDR environment variables are set")
}

func getVaultToken() string {
	if token := os.Getenv("VAULT_SERVER_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("VAULT_TOKEN"); token != "" {
		return token
	}
	return ""
}

type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultDialer) OpenVariableURL(ctx context.Context, u *url.URL) (*runtimevar.Variable, error) {
	o.init.Do(func() {
		serverURL, err := getVaultURL()
		if err != nil {
			o.err = err
			return
		}
		token := getVaultToken()
		cfg := Config{Token: token, APIConfig: api.Config{Address: serverURL}}
		client, err := Dial(ctx, &cfg)
		if err != nil {
			o.err = fmt.Errorf("failed to Dial default Vault server at %q: %v", serverURL, err)
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open variable %v: %v", u, o.err)
	}
	return o.opener.OpenVariableURL(ctx, u)
}

// URLOpener opens Vault URLs like "hashivault://myapp/config".
//
// The URL host+path are used as the secret path.
//
// The following query parameters are supported:
//   - decoder: The decoder to use. Defaults to runtimevar.BytesDecoder.
//     See runtimevar.DecoderByName for supported values.
//   - wait: The poll interval, in time.ParseDuration formats. Defaults to 30s.
//   - engine_version: The KV engine version (1 or 2). Defaults to 2.
//   - mount: The KV mount path. Defaults to "secret".
type URLOpener struct {
	// Client must be non-nil.
	Client *api.Client

	// Decoder specifies the decoder to use if one is not specified in the URL.
	// Defaults to runtimevar.BytesDecoder.
	Decoder *runtimevar.Decoder

	// Options specifies the options to pass to OpenVariable.
	Options Options
}

// OpenVariableURL opens a hashivault Variable for u.
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

	if s := q.Get("engine_version"); s != "" {
		q.Del("engine_version")
		v, err := strconv.Atoi(s)
		if err != nil || (v != 1 && v != 2) {
			return nil, fmt.Errorf("open variable %v: invalid engine_version %q: must be 1 or 2", u, s)
		}
		opts.EngineVersion = v
	}

	if s := q.Get("mount"); s != "" {
		q.Del("mount")
		opts.Mount = s
	}

	for param := range q {
		return nil, fmt.Errorf("open variable %v: invalid query parameter %q", u, param)
	}

	secretPath := path.Join(u.Host, u.Path)

	return OpenVariable(o.Client, secretPath, decoder, &opts)
}

// Options sets options for constructing a *runtimevar.Variable backed by Vault.
type Options struct {
	// WaitDuration controls the rate at which the Vault server is polled.
	// Defaults to 30 seconds.
	WaitDuration time.Duration

	// EngineVersion specifies the KV secrets engine version.
	// Valid values are 1 and 2. Defaults to 2 (modern KV v2).
	EngineVersion int

	// Mount is the mount path of the KV secrets engine.
	// Defaults to "secret".
	Mount string
}

// OpenVariable constructs a *runtimevar.Variable that uses client to read the
// variable at the given path from a Vault KV secrets engine.
//
// The path should be the path to the secret without the mount prefix or
// "data" segment (for KV v2). For example, "myapp/config".
func OpenVariable(client *api.Client, secretPath string, decoder *runtimevar.Decoder, opts *Options) (*runtimevar.Variable, error) {
	w, err := newWatcher(client, secretPath, decoder, opts)
	if err != nil {
		return nil, err
	}
	return runtimevar.New(w), nil
}

func newWatcher(client *api.Client, secretPath string, decoder *runtimevar.Decoder, opts *Options) (driver.Watcher, error) {
	if opts == nil {
		opts = &Options{}
	}

	engineVersion := opts.EngineVersion
	if engineVersion == 0 {
		engineVersion = 2
	}
	if engineVersion != 1 && engineVersion != 2 {
		return nil, fmt.Errorf("invalid engine_version %d; must be 1 or 2", engineVersion)
	}

	mount := opts.Mount
	if mount == "" {
		mount = "secret"
	}

	var fullPath string
	if engineVersion == 2 {
		fullPath = path.Join(mount, "data", secretPath)
	} else {
		fullPath = path.Join(mount, secretPath)
	}

	return &watcher{
		client:        client,
		path:          fullPath,
		decoder:       decoder,
		wait:          driver.WaitDuration(opts.WaitDuration),
		engineVersion: engineVersion,
		mount:         mount,
	}, nil
}

type state struct {
	val        any
	raw        *api.Secret
	rawBytes   []byte
	updateTime time.Time
	err        error
}

// Value implements driver.State.Value.
func (s *state) Value() (any, error) {
	return s.val, s.err
}

// UpdateTime implements driver.State.UpdateTime.
func (s *state) UpdateTime() time.Time {
	return s.updateTime
}

// As implements driver.State.As.
func (s *state) As(i any) bool {
	if s.raw == nil {
		return false
	}
	p, ok := i.(**api.Secret)
	if !ok {
		return false
	}
	*p = s.raw
	return true
}

func errorState(err error, prevS driver.State) driver.State {
	s := &state{err: err}
	if prevS == nil {
		return s
	}
	prev := prevS.(*state)
	if prev.err == nil {
		return s
	}
	if equivalentError(err, prev.err) {
		return nil
	}
	return s
}

func equivalentError(err1, err2 error) bool {
	if err1 == err2 || err1.Error() == err2.Error() {
		return true
	}
	var secErr1, secErr2 *SecretError
	if errors.As(err1, &secErr1) && errors.As(err2, &secErr2) {
		return secErr1.Code == secErr2.Code
	}
	var respErr1, respErr2 *api.ResponseError
	if errors.As(err1, &respErr1) && errors.As(err2, &respErr2) {
		return respErr1.StatusCode == respErr2.StatusCode
	}
	return false
}

type watcher struct {
	client        *api.Client
	path          string
	decoder       *runtimevar.Decoder
	wait          time.Duration
	engineVersion int
	mount         string
}

// WatchVariable implements driver.WatchVariable.
func (w *watcher) WatchVariable(ctx context.Context, prev driver.State) (driver.State, time.Duration) {
	secret, err := w.client.Logical().ReadWithContext(ctx, w.path)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	if secret == nil {
		return errorState(newNotFoundError(w.path), prev), w.wait
	}

	var data map[string]any
	if w.engineVersion == 2 {
		dataRaw, ok := secret.Data["data"]
		if !ok {
			return errorState(newInvalidDataError(w.path, "no data field in KV v2 response"), prev), w.wait
		}
		data, ok = dataRaw.(map[string]any)
		if !ok {
			return errorState(newInvalidDataError(w.path, "invalid data format"), prev), w.wait
		}
	} else {
		data = secret.Data
	}

	var rawBytes []byte
	if len(data) == 1 {
		if v, ok := data["value"]; ok {
			switch val := v.(type) {
			case string:
				rawBytes = []byte(val)
			case []byte:
				rawBytes = val
			default:
				rawBytes, err = json.Marshal(data)
				if err != nil {
					return errorState(err, prev), w.wait
				}
			}
		} else {
			rawBytes, err = json.Marshal(data)
			if err != nil {
				return errorState(err, prev), w.wait
			}
		}
	} else {
		rawBytes, err = json.Marshal(data)
		if err != nil {
			return errorState(err, prev), w.wait
		}
	}

	if prev != nil {
		if prevState, ok := prev.(*state); ok && bytes.Equal(rawBytes, prevState.rawBytes) {
			return nil, w.wait
		}
	}

	val, err := w.decoder.Decode(ctx, rawBytes)
	if err != nil {
		return errorState(err, prev), w.wait
	}

	return &state{
		val:        val,
		raw:        secret,
		rawBytes:   rawBytes,
		updateTime: time.Now(),
	}, w.wait
}

// Close implements driver.Close.
func (w *watcher) Close() error {
	return nil
}

// ErrorAs implements driver.ErrorAs.
func (w *watcher) ErrorAs(err error, i any) bool {
	var secErr *SecretError
	if errors.As(err, &secErr) {
		if p, ok := i.(**SecretError); ok {
			*p = secErr
			return true
		}
	}
	var respErr *api.ResponseError
	if errors.As(err, &respErr) {
		if p, ok := i.(**api.ResponseError); ok {
			*p = respErr
			return true
		}
	}
	return false
}

// ErrorCode implements driver.ErrorCode.
func (w *watcher) ErrorCode(err error) gcerrors.ErrorCode {
	var secErr *SecretError
	if errors.As(err, &secErr) {
		switch secErr.Code {
		case 400:
			return gcerr.InvalidArgument
		case 403:
			return gcerr.PermissionDenied
		case 404:
			return gcerr.NotFound
		case 429, 503:
			return gcerr.ResourceExhausted
		case 500, 502:
			return gcerr.Internal
		}
	}
	var respErr *api.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.StatusCode {
		case 400:
			return gcerr.InvalidArgument
		case 403:
			return gcerr.PermissionDenied
		case 404:
			return gcerr.NotFound
		case 429, 503:
			return gcerr.ResourceExhausted
		case 500, 502:
			return gcerr.Internal
		}
	}
	return gcerr.Unknown
}
