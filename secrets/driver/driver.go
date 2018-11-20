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

// Package driver provides the interface for providers of secrets package. This
// serves as a contract of how the secrets API uses a provider implementation.
package driver

import "context"

// Decryptor decrypts a cipher message into a plain text message. The
// implementation of the Decryptor should hold any key information with it, for
// example, the key name. The information should be get during the creation. If
// user wants to decrypt a message with a different key, they would create
// another Decryptor.
type Decryptor interface {

	// Decrypt decrypts the ciphertext and returns the plaintext. It should handle
	// any necessary base64 decoding based on the service provider implementation.
	// If an RPC call or any other key information is needed, it should use those
	// found in the Decryptor.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}
