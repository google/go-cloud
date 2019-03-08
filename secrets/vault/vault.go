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
// limtations under the License.

// Package vault provides a secrets implementation using the Transit Secrets
// Engine of Vault by Hashicorp.
// Use NewKeeper to construct a *secrets.Keeper.
//
// URLs
//
// For secrets.OpenKeeper URLs, vault registers for the scheme "vault"; URLs
// start with "vault://". secrets.OpenKeeper will dial a Vault server once per
// unique combination of the following supported URL parameters:
//   - address: Sets Config.APIConfig.Address; should be a full URL with the
//       address of the Vault server.
//   - token: Sets Config.Token; the access token the Vault client will use.
// Example URL: "vault://mykey?address=http://vault.server.com:8080&token=aaaaa".
//
// As
//
// vault does not support any types for As.
package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/gcerrors"
	"gocloud.dev/secrets"
)

// Config is the authentication configurations of the Vault server.
type Config struct {
	// Token is the access token the Vault client uses to talk to the server.
	// See https://www.vaultproject.io/docs/concepts/tokens.html for more
	// information.
	Token string
	// APIConfig is used to configure the creation of the client.
	APIConfig api.Config
}

// Dial gets a Vault client.
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

func init() {
	secrets.DefaultURLMux().Register(Scheme, new(lazyDialer))
}

// lazyDialer lazily dials unique Vault servers.
type lazyDialer struct {
	mu      sync.Mutex
	clients map[string]*api.Client
}

func (o *lazyDialer) cachedClient(ctx context.Context, u *url.URL) (*api.Client, *url.URL, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.clients == nil {
		o.clients = map[string]*api.Client{}
	}
	var cfg Config
	var cacheKeyParts []string
	q := u.Query()
	for param, values := range u.Query() {
		value := values[0]
		switch param {
		case "token":
			cfg.Token = value
		case "address":
			cfg.APIConfig.Address = value
		default:
			continue
		}
		cacheKeyParts = append(cacheKeyParts, fmt.Sprintf("%s=%s", param, value))
		q.Del(param)
	}
	sort.Strings(cacheKeyParts)
	cacheKey := strings.Join(cacheKeyParts, ",")
	client := o.clients[cacheKey]
	if client == nil {
		var err error
		client, err = Dial(ctx, &cfg)
		if err != nil {
			return nil, nil, err
		}
		o.clients[cacheKey] = client
	}
	// Returned an updated URL with the query parameters that we used cleared.
	u2 := u
	u2.RawQuery = q.Encode()
	return client, u2, nil
}

func (o *lazyDialer) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	client, u2, err := o.cachedClient(ctx, u)
	if err != nil {
		return nil, err
	}
	opener := &URLOpener{Client: client}
	return opener.OpenKeeperURL(ctx, u2)
}

// Scheme is the URL scheme vault registers its URLOpener under on secrets.DefaultMux.
const Scheme = "vault"

// URLOpener opens Vault URLs like "vault://mykey".
// The URL Host + Path are used as the keyID.
type URLOpener struct {
	// Client must be non-nil.
	Client *api.Client

	// Options specifies the default options to pass to NewKeeper.
	Options KeeperOptions
}

// OpenKeeperURL opens the Keeper URL.
func (o *URLOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open keeper %q: invalid query parameter %q", u, param)
	}
	return NewKeeper(o.Client, path.Join(u.Host, u.Path), &o.Options), nil
}

// NewKeeper returns a *secrets.Keeper that uses the Transit Secrets Engine of
// Vault by Hashicorp.
// See the package documentation for an example.
func NewKeeper(client *api.Client, keyID string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		keyID:  keyID,
		client: client,
	})
}

type keeper struct {
	// keyID is an encryption key ring name used by the Vault's transit API.
	keyID  string
	client *api.Client
}

// Decrypt decrypts the ciphertext into a plaintext.
func (k *keeper) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	out, err := k.client.Logical().Write(
		path.Join("transit/decrypt", k.keyID),
		map[string]interface{}{
			"ciphertext": string(ciphertext),
		},
	)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data["plaintext"].(string))
}

// Encrypt encrypts a plaintext into a ciphertext.
func (k *keeper) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	secret, err := k.client.Logical().Write(
		path.Join("transit/encrypt", k.keyID),
		map[string]interface{}{
			"plaintext": plaintext,
		},
	)
	if err != nil {
		return nil, err
	}
	return []byte(secret.Data["ciphertext"].(string)), nil
}

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i interface{}) bool {
	return false
}

// ErrorCode implements driver.ErrorCode.
func (k *keeper) ErrorCode(error) gcerrors.ErrorCode {
	// TODO(shantuo): try to classify vault error codes
	return gcerrors.Unknown
}

// KeeperOptions controls Keeper behaviors.
// It is provided for future extensibility.
type KeeperOptions struct{}
