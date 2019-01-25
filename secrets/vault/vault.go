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
package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"path"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/secrets"
)

// Config is the authentication configurations of the Vault server.
type Config struct {
	// Token is the access token the Vault client uses to talk to the server.
	// See https://www.vaultproject.io/docs/concepts/tokens.html for more
	// information.
	Token     string
	APIConfig *api.Config
}

// Dial gets a Vault client.
func Dial(ctx context.Context, cfg *Config) (*api.Client, error) {
	if cfg == nil {
		return nil, errors.New("no auth Config provided")
	}
	c, err := api.NewClient(cfg.APIConfig)
	if err != nil {
		return nil, err
	}
	if cfg.Token != "" {
		c.SetToken(cfg.Token)
	}
	return c, nil
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

// KeeperOptions controls Keeper behaviors.
// It is provided for future extensibility.
type KeeperOptions struct{}
