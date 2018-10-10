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

// Package gcpkms provides functionality to access and decrypt secrets using
// Google Cloud KMS.
package gcpkms

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/go-cloud/gcp"
	cloudkms "google.golang.org/api/cloudkms/v1"
)

// KeyInfo contains information to construct the pull path of a key.
type KeyInfo struct {
	ProjectID, Location, KeyRing, KeyID string
}

func (ki *KeyInfo) keyPath() string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		ki.ProjectID, ki.Location, ki.KeyRing, ki.KeyID)
}

// New gets a Cloud KMS service.
func New(ctx context.Context, hc *gcp.HTTPClient) (*cloudkms.Service, error) {
	if hc == nil {
		return nil, errors.New("getting KMS service: no HTTP client provided")
	}
	return cloudkms.New(&hc.Client)
}

// DecryptBase64 decrypts the base64-encoded encCiphertext using the key constructed from ki.
func DecryptBase64(ctx context.Context, kms *cloudkms.Service, ki *KeyInfo, encCiphertext string) ([]byte, error) {
	req := &cloudkms.DecryptRequest{
		Ciphertext: encCiphertext,
	}
	resp, err := kms.Projects.Locations.KeyRings.CryptoKeys.Decrypt(ki.keyPath(), req).Do()
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Plaintext)
}

// Decrypt decrypts the ciphertext using the key constructed from ki.
func Decrypt(ctx context.Context, kms *cloudkms.Service, ki *KeyInfo, ciphertext []byte) ([]byte, error) {
	return DecryptBase64(ctx, kms, ki, base64.StdEncoding.EncodeToString(ciphertext))
}
