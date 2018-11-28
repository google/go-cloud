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

// Package driver defines interfaces to be implemented for providers of the
// secrets package.
package driver

import "context"

// Decrypter decrypts a cipher message into a plain text message. The
// implementation of the Decrypter should hold any key information with it, for
// example, the key name.
type Decrypter interface {

	// Decrypt decrypts the ciphertext and returns the plaintext. It should handle
	// any necessary base64 encoding/decoding based on the service provider
	// implementation. If an RPC call or any other key information is needed, it
	// should use those found in the Decrypter.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}
