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

// Package localsigners provides a signers implementation using a locally
// provided crypto.Hash and some pepper.
//
// Use NewSigner to construct a *signers.Signer.
//
// URLs
//
// For signers.OpenSigner, localsigners registers for the scheme "base64signer".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// localsigners does not support any types for As.
package localsigners // import "gocloud.dev/signers/localsigners"

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"

	"gocloud.dev/gcerrors"
	"gocloud.dev/signers"
)

func init() {
	signers.DefaultURLMux().RegisterSigner(Scheme, &URLOpener{})
}

// Scheme is the URL scheme localsigners registers its URLOpener under on
// secrets.DefaultMux.
// See the package documentation and/or URLOpener for details.
const (
	Scheme = "base64signer"
)

// URLOpener opens localsigners URLs like
//
//    base64signer://<algorithm>/smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=
//
// The URL host MUST specify a crypto.Hash and the path must be base64 encoded
// and MUST decode to at least 32 bytes. If the URL host is empty, e.g.
//
//    base64signer:///smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=
//
// a SHA3_512 digest is used. If the URL path is empty
//
//    base64signer://<algorithm>
//
// a new random pepper is generated.
//
// No query parameters are supported.
type URLOpener struct{}

// OpenSignerURL opens Signer URLs.
func (o *URLOpener) OpenSignerURL(_ context.Context, u *url.URL) (*signers.Signer, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open signer %v: invalid query parameter %q", u, param)
	}
	var pepper []byte
	var hash crypto.Hash
	var err error
	if u.Host == "" {
		hash = crypto.SHA3_512
	} else {
		hash, err = strToHash(u.Host)
	}
	if err != nil {
		return nil, fmt.Errorf("open signer %v: failed to get algorithm: %v", u, err)
	}
	if u.Path == "" {
		pepper, err = NewRandomPepper()
	} else {
		pepper, err = Base64Pepper(u.Path[1:])
	}
	if err != nil {
		return nil, fmt.Errorf("open signer %v: failed to get key: %v", u, err)
	}
	return NewSigner(hash, pepper)
}

// signer holds a crypto.Hash and pepper for use in signing digests, and
// implements driver.Signer.
type signer struct {
	hash   crypto.Hash
	pepper []byte
}

// NewSigner returns a *signers.Signer that uses the given cryptographic hash
// function and secret
// key. See the package documentation for an example.
func NewSigner(hash crypto.Hash, pepper []byte) (*signers.Signer, error) {
	if !hash.Available() {
		return nil, fmt.Errorf("invalid hash: %s", hash)
	}
	return signers.NewSigner(
		&signer{
			hash:   hash,
			pepper: pepper,
		},
	), nil
}

// Base64Pepper takes pepper as a base64 string and converts it to a []byte. It
// uses base64.URLEncoding.  The pepper MUST be at least 32 bytes long, else an
// error is returned.
func Base64Pepper(base64str string) ([]byte, error) {
	return base64Pepper(base64str, base64.StdEncoding)
}

func base64Pepper(base64str string, encoding *base64.Encoding) ([]byte, error) {
	key, err := encoding.DecodeString(base64str)
	if err != nil {
		return nil, err
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("pepper must be greater than 32 bytes")
	}
	return key, nil
}

// NewRandomPepper will generate random secret key material suitable to be
// used as the secret key argument to NewSigner.
func NewRandomPepper() ([]byte, error) {
	var pepper [32]byte
	// Read random numbers into the passed slice until it's full.
	_, err := rand.Read(pepper[:])
	if err != nil {
		return pepper[:], err
	}
	return pepper[:], nil
}

// Sign creates a signature of a digest using the configured crypto.Hash and
// pepper.
func (k *signer) Sign(_ context.Context, digest []byte) ([]byte, error) {
	hasher := k.hash.New()
	_, err := fmt.Fprint(hasher, digest)
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprint(hasher, k.pepper)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

// Verify verifies a signature using the configured key.
func (k *signer) Verify(ctx context.Context, digest []byte, signature []byte) (bool, error) {
	expected, err := k.Sign(ctx, digest)
	if err != nil {
		return false, err
	}
	return bytes.Equal(expected, signature), nil
}

// Close implements driver.Signer.Close.
func (k *signer) Close() error { return nil }

// ErrorAs implements driver.Signer.ErrorAs.
func (k *signer) ErrorAs(_ error, _ interface{}) bool {
	return false
}

// ErrorCode implements driver.ErrorCode.
func (k *signer) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }
