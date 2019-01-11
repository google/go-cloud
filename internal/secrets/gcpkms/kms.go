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
// limtations under the License.

// Package gcpkms provides functionality to encrypt and decrypt secrets using
// Google Cloud KMS.
package gcpkms

import (
	"context"
	"fmt"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/secrets"
	"google.golang.org/api/option"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
)

// endPoint is the address to access Google Cloud KMS API.
const endPoint = "cloudkms.googleapis.com:443"

// Dial returns a client to use with Cloud KMS and a clean-up function to close
// the client after used.
func Dial(ctx context.Context, ts gcp.TokenSource) (*cloudkms.KeyManagementClient, func(), error) {
	c, err := cloudkms.NewKeyManagementClient(ctx, option.WithTokenSource(ts))
	return c, func() { c.Close() }, err
}

// NewCrypter returns a new Crypter to to encryption and decryption.
func NewCrypter(client *cloudkms.KeyManagementClient, ki *KeyID) *secrets.Crypter {
	return secrets.NewCrypter(&crypter{
		keyID:  ki,
		client: client,
	}, nil)
}

// KeyID includes related information to construct a key name that is managed
// by Cloud KMS.
// See https://cloud.google.com/kms/docs/object-hierarchy#key for more
// information.
type KeyID struct {
	ProjectID, Location, KeyRing, Key string
}

func (ki *KeyID) String() string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		ki.ProjectID, ki.Location, ki.KeyRing, ki.Key)
}

// crypter contains information to construct the pull path of a key.
type crypter struct {
	keyID  *KeyID
	client *cloudkms.KeyManagementClient
}

// Decrypt decrypts the ciphertext using the key constructed from ki.
func (c *crypter) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	req := &kmspb.DecryptRequest{
		Name:       c.keyID.String(),
		Ciphertext: ciphertext,
	}
	resp, err := c.client.Decrypt(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetPlaintext(), nil
}

// Encrypt encrypts the plaintext into a ciphertext.
func (c *crypter) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	req := &kmspb.EncryptRequest{
		Name:      c.keyID.String(),
		Plaintext: plaintext,
	}
	resp, err := c.client.Encrypt(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetCiphertext(), nil
}
