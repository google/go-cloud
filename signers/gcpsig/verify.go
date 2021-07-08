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
// limitations under the License.

package gcpsig

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/asn1"
	"fmt"
	"math/big"

	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
)

type verifier interface {
	Verify(pubKey interface{}, hash crypto.Hash, digest []byte, signature []byte) (bool, error)
}

type rsaVerifier struct{}

func (v *rsaVerifier) Verify(pubKey interface{}, hash crypto.Hash, digest []byte, signature []byte) (bool, error) {
	rsaKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return false, fmt.Errorf("public key is not rsa")
	}

	if err := rsa.VerifyPSS(rsaKey, hash, digest[:], signature, &rsa.PSSOptions{
		SaltLength: len(digest),
		Hash:       crypto.SHA512,
	}); err != nil {
		return false, fmt.Errorf("failed to verify signature: %v", err)
	}
	return true, nil
}

type ecVerifier struct{}

func (v *ecVerifier) Verify(pubKey interface{}, _ crypto.Hash, digest []byte, signature []byte) (bool, error) {
	ecKey, ok := pubKey.(*ecdsa.PublicKey)
	if !ok {
		return false, fmt.Errorf("public key is not rsa")
	}

	// Verify Elliptic Curve signature.
	var parsedSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(signature, &parsedSig); err != nil {
		return false, fmt.Errorf("failed to unmarshall ASN1 signature: %v", err)
	}

	if !ecdsa.Verify(ecKey, digest[:], parsedSig.R, parsedSig.S) {
		return false, fmt.Errorf("failed to verify signature")
	}
	return true, nil
}

func algorithmToVerifier(algorithm kmspb.CryptoKeyVersion_CryptoKeyVersionAlgorithm) (verifier, error) {
	switch algorithm {
	case kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PSS_3072_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_3072_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA256,
		kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_2048_SHA256,
		kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_3072_SHA256,
		kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_4096_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA512,
		kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA512,
		kmspb.CryptoKeyVersion_RSA_DECRYPT_OAEP_4096_SHA512:
		return &rsaVerifier{}, nil
	case kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256,
		kmspb.CryptoKeyVersion_EC_SIGN_P384_SHA384:
		return &ecVerifier{}, nil
	}
	return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
}
