// Copyright 2019 The Go Cloud Authors
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

// Package vault provides functionality to access and decrypt secrets using
// Hashicorp Vault transit API.
package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"path"

	"github.com/hashicorp/vault/api"
	"gocloud.dev/secrets"
)

const endpointPrefix = "/v1/transit"

// Config is the authentication configurations of the Vault server.
type Config struct {
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

// NewKeeper returns a new Keeper to do encryption and decryption.
func NewKeeper(client *api.Client, keyID string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		keyID:  keyID,
		client: client,
	})
}

type keeper struct {
	keyID  string
	client *api.Client
}

// Decrypt decrypts the ciphertext into a plaintext.
func (k *keeper) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	r := k.client.NewRequest(http.MethodPost, path.Join(endpointPrefix, "decrypt", k.keyID))
	// Use string here to avoid being base64-encoded, Vault API only takes
	// ciphertext in its original form.
	var cipher = struct {
		Ciphertext string `json:"ciphertext"`
	}{
		Ciphertext: string(ciphertext),
	}

	var err error
	r.BodyBytes, err = json.Marshal(cipher)
	if err != nil {
		return nil, err
	}

	res, err := k.client.RawRequestWithContext(ctx, r)
	if err != nil {
		return nil, err
	}

	var decrypted struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	res.DecodeJSON(&decrypted)
	res.Body.Close()
	return base64.StdEncoding.DecodeString(decrypted.Data.Plaintext)
}

// Encrypt encrypts a plaintext into a ciphertext.
func (k *keeper) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	r := k.client.NewRequest(http.MethodPost, path.Join(endpointPrefix, "encrypt", k.keyID))
	var plain struct {
		Plaintext string `json:"plaintext"`
	}
	plain.Plaintext = base64.StdEncoding.EncodeToString(plaintext)

	var err error
	r.BodyBytes, err = json.Marshal(plain)
	if err != nil {
		return nil, err
	}
	res, err := k.client.RawRequestWithContext(ctx, r)
	if err != nil {
		return nil, err
	}

	var encrypted struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	res.DecodeJSON(&encrypted)
	res.Body.Close()
	return []byte(encrypted.Data.Ciphertext), nil
}

// KeeperOptions controls Keeper behaviors.
// It is provided for future extensibility.
type KeeperOptions struct{}
