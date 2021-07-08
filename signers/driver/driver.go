// Copyright 2021 The Go Cloud Development Kit Authors
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
//  limitations under the License.
//

// Package driver defines interfaces to be implemented by signers drivers,
// which will be used by the signers package to interact with the underlying
// services. Application code should use package signers.
package driver // import "gocloud.dev/signers/driver"

import (
	"context"

	"gocloud.dev/gcerrors"
)

// Signer holds the key information to sign digests as well as verify
// signers of digests.
type Signer interface {

	// Sign creates a signature of a digest using the configured key. The sign
	// operation supports both asymmetric and symmetric keys.
	//
	// In the case of asymmetric keys, the digest must be produced with the
	// same digest algorithm as specified by the key.
	Sign(ctx context.Context, digest []byte) (signature []byte, err error)

	// Verify verifies a signature using the configured key. The verify
	// operation supports both symmetric keys and asymmetric keys.
	//
	// In case of asymmetric keys public portion of the key is used to verify
	// the signature.  Also, the digest must be produced with the same digest
	// algorithm as specified by the key.
	Verify(ctx context.Context, digest []byte, signature []byte) (ok bool, err error)

	// Close releases any resources used for the Signer.
	Close() error

	// ErrorAs allows drivers to expose driver-specific types for returned
	// errors.
	//
	// See https://gocloud.dev/concepts/as/ for background information.
	ErrorAs(err error, i interface{}) bool

	// ErrorCode should return a code that describes the error, which was returned
	// by one of the other methods in this interface.
	ErrorCode(error) gcerrors.ErrorCode
}
