// Copyright 2018 The Go Cloud Development Kit Authors
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

// Package localsecrets provides a secrets implementation using a locally
// provided symmetric key.
// Use NewKeeper to construct a *secrets.Keeper.
//
// # URLs
//
// For secrets.OpenKeeper, localsecrets registers for the scheme "base64key".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// # As
//
// localsecrets does not support any types for As.
package localsecrets // import "gocloud.dev/secrets/localsecrets"

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"gocloud.dev/gcerrors"
	"gocloud.dev/secrets"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/nacl/secretbox"
)

func init() {
	secrets.DefaultURLMux().RegisterKeeper(Scheme, &URLOpener{})
}

// Scheme is the URL scheme localsecrets registers its URLOpener under on
// secrets.DefaultMux.
// See the package documentation and/or URLOpener for details.
const (
	Scheme = "base64key"
)

// URLOpener opens localsecrets URLs like "base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=".
//
// The URL host must be base64 encoded, and must decode to exactly 32 bytes.
// Note that base64.URLEncoding should be used to avoid URL-unsafe character in the hostname.
// If the URL host is empty (e.g., "base64key://"), a new random key is generated.
//
// EncryptionContext key/value pairs can be provided by providing URL parameters prefixed
// with "context_"; e.g., "...&context_abc=foo&context_def=bar" would result in
// an EncryptionContext of {abc=foo, def=bar}.
//
// A "salt" query parameter can be provided for HKDF domain separation when using
// EncryptionContext; e.g., "...&salt=myapp". If not set, the default salt
// "gocloud.dev/secrets/localsecrets" is used.
type URLOpener struct {
	// Options specifies the options to pass to OpenKeeper.
	// EncryptionContext parameters from the URL are merged in.
	Options KeeperOptions
}

// OpenKeeperURL opens Keeper URLs.
func (o *URLOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	queryParams := u.Query()
	opts := o.Options
	if err := addEncryptionContextFromURLParams(&opts, queryParams); err != nil {
		return nil, err
	}
	if s := queryParams.Get("salt"); s != "" {
		opts.Salt = []byte(s)
		queryParams.Del("salt")
	}
	for param := range queryParams {
		return nil, fmt.Errorf("open keeper %v: invalid query parameter %q", u, param)
	}
	var sk [32]byte
	var err error
	if u.Host == "" {
		sk, err = NewRandomKey()
	} else {
		sk, err = Base64Key(u.Host)
	}
	if err != nil {
		return nil, fmt.Errorf("open keeper %v: failed to get key: %v", u, err)
	}
	return OpenKeeper(sk, &opts), nil
}

// addEncryptionContextFromURLParams merges any EncryptionContext URL parameters from
// u into opts.EncryptionContext.
// It removes the processed URL parameters from u.
func addEncryptionContextFromURLParams(opts *KeeperOptions, u url.Values) error {
	for k, vs := range u {
		if strings.HasPrefix(k, "context_") {
			if len(vs) != 1 {
				return fmt.Errorf("open keeper: EncryptionContext URL parameters %q must have exactly 1 value", k)
			}
			u.Del(k)
			if opts.EncryptionContext == nil {
				opts.EncryptionContext = map[string]string{}
			}
			opts.EncryptionContext[k[8:]] = vs[0]
		}
	}
	return nil
}

// keeper holds a secret for use in symmetric encryption,
// and implements driver.Keeper.
type keeper struct {
	secretKey [32]byte // secretbox key size
	opts      KeeperOptions
}

// OpenKeeper returns a *secrets.Keeper that uses the given symmetric
// key with the given options.
func OpenKeeper(sk [32]byte, opts *KeeperOptions) *secrets.Keeper {
	if opts == nil {
		opts = &KeeperOptions{}
	}
	return secrets.NewKeeper(&keeper{secretKey: sk, opts: *opts})
}

// NewKeeper returns a *secrets.Keeper that uses the given symmetric
// key. See the package documentation for an example.
func NewKeeper(sk [32]byte) *secrets.Keeper {
	return OpenKeeper(sk, nil)
}

// KeeperOptions controls Keeper behaviors.
type KeeperOptions struct {
	// EncryptionContext is a set of key/value pairs that are used during
	// encryption and decryption. The same EncryptionContext must be provided
	// for both operations; decryption will fail if the context does not match.
	// See https://docs.aws.amazon.com/kms/latest/developerguide/concepts.html#encrypt_context.
	EncryptionContext map[string]string
	// Salt is used as the HKDF salt for key derivation when EncryptionContext
	// is set. It provides domain separation so that different applications using
	// the same key and context derive different subkeys.
	// If nil, defaults to "gocloud.dev/secrets/localsecrets".
	Salt []byte
}

var defaultSalt = []byte("gocloud.dev/secrets/localsecrets")

// deriveKey derives a [32]byte key from the base secret key and the encryption context
// using HKDF (RFC 5869). If no encryption context is set, the base key is returned unchanged.
func deriveKey(sk [32]byte, opts KeeperOptions) [32]byte {
	if len(opts.EncryptionContext) == 0 {
		return sk
	}
	// Build deterministic info from sorted context key/value pairs
	// with null byte separators to prevent ambiguity.
	keys := make([]string, 0, len(opts.EncryptionContext))
	for k := range opts.EncryptionContext {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var info []byte
	for _, k := range keys {
		info = append(info, k...)
		info = append(info, 0)
		info = append(info, opts.EncryptionContext[k]...)
		info = append(info, 0)
	}
	salt := opts.Salt
	if salt == nil {
		salt = defaultSalt
	}
	r := hkdf.New(sha256.New, sk[:], salt, info)
	var derived [32]byte
	if _, err := io.ReadFull(r, derived[:]); err != nil {
		// sha256-based HKDF will never fail to produce 32 bytes.
		panic(err)
	}
	return derived
}

// Base64KeyStd takes a secret key as a base64 string and converts it
// to a [32]byte, erroring if the decoded data is not 32 bytes.
// It uses base64.StdEncoding.
func Base64KeyStd(base64str string) ([32]byte, error) {
	return base64Key(base64str, base64.StdEncoding)
}

// Base64Key takes a secret key as a base64 string and converts it
// to a [32]byte, erroring if the decoded data is not 32 bytes.
// It uses base64.URLEncoding.
func Base64Key(base64str string) ([32]byte, error) {
	return base64Key(base64str, base64.URLEncoding)
}

func base64Key(base64str string, encoding *base64.Encoding) ([32]byte, error) {
	var sk32 [32]byte
	key, err := encoding.DecodeString(base64str)
	if err != nil {
		return sk32, err
	}
	keySize := len([]byte(key))
	if keySize != 32 {
		return sk32, fmt.Errorf("Base64Key: secret key material is %v bytes, want 32 bytes", keySize)
	}
	copy(sk32[:], key)
	return sk32, nil
}

// NewRandomKey will generate random secret key material suitable to be
// used as the secret key argument to NewKeeper.
func NewRandomKey() ([32]byte, error) {
	var sk32 [32]byte
	// Read random numbers into the passed slice until it's full.
	_, err := rand.Read(sk32[:])
	if err != nil {
		return sk32, err
	}
	return sk32, nil
}

const nonceSize = 24

// Encrypt encrypts a message using a per-message generated nonce and
// the secret held in the Keeper.
func (k *keeper) Encrypt(ctx context.Context, message []byte) ([]byte, error) {
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	// secretbox.Seal appends the encrypted message to its first argument and returns
	// the result; using a slice on top of the nonce array for this "out" arg allows reading
	// the nonce out of the first nonceSize bytes when the message is decrypted.
	key := deriveKey(k.secretKey, k.opts)
	return secretbox.Seal(nonce[:], message, &nonce, &key), nil
}

// Decrypt decrypts a message using a nonce that is read out of the first nonceSize bytes
// of the message and a secret held in the Keeper.
func (k *keeper) Decrypt(ctx context.Context, message []byte) ([]byte, error) {
	if len(message) < nonceSize {
		return nil, fmt.Errorf("localsecrets: invalid message length (%d, expected at least %d)", len(message), nonceSize)
	}
	var decryptNonce [nonceSize]byte
	copy(decryptNonce[:], message[:nonceSize])

	key := deriveKey(k.secretKey, k.opts)
	decrypted, ok := secretbox.Open(nil, message[nonceSize:], &decryptNonce, &key)
	if !ok {
		return nil, errors.New("localsecrets: Decrypt failed")
	}
	return decrypted, nil
}

// Close implements driver.Keeper.Close.
func (k *keeper) Close() error { return nil }

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i any) bool {
	return false
}

// ErrorCode implements driver.ErrorCode.
func (k *keeper) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }
