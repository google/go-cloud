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

// Package gcpsig provides a signers implementation backed by Google Cloud KMS.
// Use OpenSigner to construct a *signers.Signer.
//
// URLs
//
// For signers.OpenSigner, gcpsig registers for the scheme "gcpsig".
// The default URL opener will create a connection using use default
// credentials from the environment, as described in
// https://cloud.google.com/docs/authentication/production.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// gcpsig exposes the following type for As:
//  - Error: *google.golang.org/grpc/status.Status
package gcpsig // import "gocloud.dev/signers/gcpsig"

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"sync"

	cloudkms "cloud.google.com/go/kms/apiv1"
	"github.com/google/wire"
	"google.golang.org/api/option"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
	"google.golang.org/grpc/status"

	"gocloud.dev/gcerrors"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/signers"
	"gocloud.dev/signers/internal"
)

// endPoint is the address to access Google Cloud KMS API.
const endPoint = "cloudkms.googleapis.com:443"

// Dial returns a client to use with Cloud KMS and a clean-up function to close
// the client after used.
func Dial(ctx context.Context, ts gcp.TokenSource) (*cloudkms.KeyManagementClient, func(), error) {
	c, err := cloudkms.NewKeyManagementClient(ctx, option.WithTokenSource(ts), useragent.ClientOption("signers"))
	return c, func() { _ = c.Close() }, err
}

func init() {
	signers.DefaultURLMux().RegisterSigner(Scheme, new(lazyCredsOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	wire.Struct(new(URLOpener), "Client"),
)

// lazyCredsOpener obtains Application Default Credentials on the first call
// to OpenSignerURL.
type lazyCredsOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazyCredsOpener) OpenSignerURL(ctx context.Context, u *url.URL) (*signers.Signer, error) {
	o.init.Do(func() {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			o.err = err
			return
		}
		client, _, err := Dial(ctx, creds.TokenSource)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open signer %v: %v", u, o.err)
	}
	return o.opener.OpenSignerURL(ctx, u)
}

// Scheme is the URL scheme gcpsig registers its URLOpener under on signers.DefaultMux.
const Scheme = "gcpsig"

// URLOpener opens GCP KMS URLs like
//
//     gcpsig://[ALGORITHM]/projects/[PROJECT_ID]/locations/[LOCATION]/keyRings/[KEY_RING]/cryptoKeys/[KEY]
//
// The URL host is used to specify the hashing ALGORITHM used.
//
// The URL path is used as the key resource ID; see
// https://cloud.google.com/kms/docs/object-hierarchy#key for more details.
//
// No query parameters are supported.
type URLOpener struct {
	// Client must be non-nil and be authenticated with "cloudkms" scope or equivalent.
	Client *cloudkms.KeyManagementClient

	// Options specifies the default options to pass to OpenSigner.
	Options SignerOptions
}

// OpenSignerURL opens the GCP KMS URLs.
func (o *URLOpener) OpenSignerURL(ctx context.Context, u *url.URL) (*signers.Signer, error) {
	for param := range u.Query() {
		return nil, fmt.Errorf("open signer %v: invalid query parameter %q", u, param)
	}
	return OpenSigner(o.Client, u.Host, u.Path, &o.Options)
}

// OpenSigner returns a *signers.Signer that uses Google Cloud KMS.
// You can use KeyResourceID to construct keyResourceID from its parts,
// or provide the whole string if you have it (e.g., from the GCP console).
// See https://cloud.google.com/kms/docs/object-hierarchy#key for more details.
// See the package documentation for an example.
func OpenSigner(client *cloudkms.KeyManagementClient, algorithm string, keyResourceID string, opts *SignerOptions) (*signers.Signer, error) {
	digester, err := internal.DigesterFromString(algorithm)
	if err != nil {
		return nil, err
	}
	return signers.NewSigner(&signer{
		digester:      digester,
		keyResourceID: keyResourceID,
		client:        client,
	}), nil
}

// KeyResourceID constructs a key resourceID for GCP KMS.
// See https://cloud.google.com/kms/docs/object-hierarchy#key for more details.
func KeyResourceID(projectID, location, keyRing, key string, version string) string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s/cryptoKeyVersions/%s",
		projectID, location, keyRing, key, version)
}

// signer implements driver.Signer.
type signer struct {
	pubKey        *kmspb.PublicKey
	digester      internal.Digester
	keyResourceID string
	client        *cloudkms.KeyManagementClient
}

// Sign creates a signature of a digest using the configured key. The sign
// operation supports both asymmetric and symmetric keys.
//
// In the case of asymmetric keys, the digest must be produced with the
// same digest algorithm as specified by the key.
func (k *signer) Sign(ctx context.Context, digest []byte) ([]byte, error) {
	req := &kmspb.AsymmetricSignRequest{
		Name:   k.keyResourceID,
		Digest: k.digester.Wrap(digest),
	}

	resp, err := k.client.AsymmetricSign(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.GetSignature(), nil
}

// Verify verifies a signature using the configured key. The verify
// operation supports both symmetric keys and asymmetric keys.
//
// In case of asymmetric keys public portion of the key is used to verify
// the signature.  Also, the digest must be produced with the same digest
// algorithm as specified by the key.
func (k *signer) Verify(ctx context.Context, digest []byte, signature []byte) (bool, error) {
	resp, err := k.getPublicKey(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get public key: %v", err)
	}

	block, _ := pem.Decode([]byte(resp.Pem))
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse public key: %v", err)
	}

	verifier, err := algorithmToVerifier(resp.Algorithm)
	if err != nil {
		return false, err
	}
	return verifier.Verify(pubKey, k.digester.HashFunc(), digest, signature)
}

func (k *signer) getPublicKey(ctx context.Context) (*kmspb.PublicKey, error) {
	if k.pubKey == nil {
		pubKey, err := k.client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{Name: k.keyResourceID})
		if err != nil {
			return nil, fmt.Errorf("failed to get public key: %v", err)
		}
		k.pubKey = pubKey
	}
	return k.pubKey, nil
}

// Close implements driver.Signer.Close.
func (k *signer) Close() error {
	return k.client.Close()
}

// ErrorAs implements driver.Signer.ErrorAs.
func (k *signer) ErrorAs(err error, i interface{}) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	p, ok := i.(**status.Status)
	if !ok {
		return false
	}
	*p = s
	return true
}

// ErrorCode implements driver.ErrorCode.
func (k *signer) ErrorCode(err error) gcerrors.ErrorCode {
	return gcerr.GRPCCode(err)
}

// SignerOptions controls Signer behaviors.
// It is provided for future extensibility.
type SignerOptions struct{}
