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

// Package azuresig provides a signers implementation backed by Azure KeyVault.
// See https://docs.microsoft.com/en-us/azure/key-vault/key-vault-whatis for
// more information. Use OpenSigner to construct a *signers.Signer.
//
// URLs
//
// For signers.OpenSigner, azuresig registers for the scheme "azuresig".
// The default URL opener will use Dial, which gets default credentials from the
// environment, unless the AZURE_KEYVAULT_AUTH_VIA_CLI environment variable is
// set to true, in which case it uses DialUsingCLIAuth to get credentials from
// the "az" command line.
//
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// azuresig exposes the following type for As:
// - Error: autorest.DetailedError, see https://godoc.org/github.com/Azure/go-autorest/autorest#DetailedError
package azuresig

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/google/wire"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/signers"
	"gocloud.dev/signers/internal"
)

var (
	// Map of HTTP Status Code to go-cloud ErrorCode
	errorCodeMap = map[int]gcerrors.ErrorCode{
		200: gcerrors.OK,
		400: gcerrors.InvalidArgument,
		401: gcerrors.PermissionDenied,
		403: gcerrors.PermissionDenied,
		404: gcerrors.NotFound,
		408: gcerrors.DeadlineExceeded,
		429: gcerrors.ResourceExhausted,
		500: gcerrors.Internal,
		501: gcerrors.Unimplemented,
	}
)

func init() {
	signers.DefaultURLMux().RegisterSigner(Scheme, new(defaultDialer))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	Dial,
	wire.Struct(new(URLOpener), "Client"),
)

// defaultDialer dials Azure KeyVault from the environment on the first call to OpenSignerURL.
type defaultDialer struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *defaultDialer) OpenSignerURL(ctx context.Context, u *url.URL) (*signers.Signer, error) {
	o.init.Do(func() {
		// Determine the dialer to use. The default one gets
		// credentials from the environment, but an alternative is
		// to get credentials from the az CLI.
		dialer := Dial
		useCLIStr := os.Getenv("AZURE_KEYVAULT_AUTH_VIA_CLI")
		if useCLIStr != "" {
			if b, err := strconv.ParseBool(useCLIStr); err != nil {
				o.err = fmt.Errorf("invalid value %q for environment variable AZURE_KEYVAULT_AUTH_VIA_CLI: %v", useCLIStr, err)
				return
			} else if b {
				dialer = DialUsingCLIAuth
			}
		}
		client, err := dialer()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{Client: client}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open signer %v: failed to Dial default KeyVault: %v", u, o.err)
	}
	return o.opener.OpenSignerURL(ctx, u)
}

// Scheme is the URL scheme azuresig registers its URLOpener under on signers.DefaultMux.
const Scheme = "azuresig"

// URLOpener opens Azure KeyVault URLs like
//
//    azuresig://{keyvault-name}.vault.azure.net/keys/{key-name}/{key-version}?algorithm=RS512
//
// The "azuresig" URL scheme is replaced with "https" to construct an Azure
// Key Vault keyID, as described in https://docs.microsoft.com/en-us/azure/key-vault/about-keys-secrets-and-certificates.
// The "/{key-version}"" suffix is optional; it defaults to the latest version.
//
// The "algorithm" query parameter sets the algorithm to use; see
// https://docs.microsoft.com/en-us/rest/api/keyvault/sign/sign#jsonwebkeysignaturealgorithm
// for supported algorithms.
//
// No other query parameters are supported.
type URLOpener struct {
	// Client must be set to a non-nil value.
	Client *keyvault.BaseClient

	// Options specifies the options to pass to OpenSigner.
	Options SignerOptions
}

// OpenSignerURL opens an Azure KeyVault Signer based on the URL u.
func (o *URLOpener) OpenSignerURL(ctx context.Context, u *url.URL) (*signers.Signer, error) {
	q := u.Query()
	algorithm := q.Get("algorithm")
	if algorithm != "" {
		o.Options.Algorithm = keyvault.JSONWebKeySignatureAlgorithm(algorithm)
		q.Del("algorithm")
	}
	for param := range q {
		return nil, fmt.Errorf("open signer %v: invalid query parameter %q", u, param)
	}
	keyID := "https://" + path.Join(u.Host, u.Path)
	return OpenSigner(o.Client, keyID, &o.Options)
}

type signer struct {
	digester    internal.Digester
	client      *keyvault.BaseClient
	keyVaultURI string
	keyName     string
	keyVersion  string
	options     *SignerOptions
}

// SignerOptions provides configuration options for sign/verify operations.
type SignerOptions struct {
	// Algorithm sets the signing algorithm used.
	// See https://docs.microsoft.com/en-us/rest/api/keyvault/sign/sign#jsonwebkeysignaturealgorithm
	// for more details.
	Algorithm keyvault.JSONWebKeySignatureAlgorithm
}

// Dial gets a new *keyvault.BaseClient using authorization from the environment.
// See https://docs.microsoft.com/en-us/go/azure/azure-sdk-go-authorization#use-environment-based-authentication.
func Dial() (*keyvault.BaseClient, error) {
	return dial(false)
}

// DialUsingCLIAuth gets a new *keyvault.BaseClient using authorization from the "az" CLI.
func DialUsingCLIAuth() (*keyvault.BaseClient, error) {
	return dial(true)
}

// dial is a helper for Dial and DialUsingCLIAuth.
func dial(useCLI bool) (*keyvault.BaseClient, error) {
	// Set the resource explicitly, because the default is the "resource manager endpoint"
	// instead of the keyvault endpoint.
	// https://azidentity.azurewebsites.net/post/2018/11/30/azure-key-vault-oauth-resource-value-https-vault-azure-net-no-slash
	// has some discussion.
	resource := os.Getenv("AZURE_AD_RESOURCE")
	if resource == "" {
		resource = "https://vault.azure.net"
	}
	authorizer := auth.NewAuthorizerFromEnvironmentWithResource
	if useCLI {
		authorizer = auth.NewAuthorizerFromCLIWithResource
	}
	a, err := authorizer(resource)
	if err != nil {
		return nil, err
	}
	client := keyvault.NewWithoutDefaults()
	client.Authorizer = a
	client.Sender = autorest.NewClientWithUserAgent(useragent.AzureUserAgentPrefix("signers"))
	return &client, nil
}

var (
	// Note that the last binding may be just a key, or key/version.
	keyIdRegex = regexp.MustCompile(`^(https://.+\.vault\.[a-z\d-.]+/)keys/(.+)$`)
)

// OpenSigner returns a *signers.Signer that uses Azure keyVault.
//
// client is a *keyvault.BaseClient instance, see https://godoc.org/github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault#BaseClient.
//
// keyID is a Azure Key Vault key identifier like "https://{keyvault-name}.vault.azure.net/keys/{key-name}/{key-version}".
// The "/{key-version}" suffix is optional; it defaults to the latest version.
// See https://docs.microsoft.com/en-us/azure/key-vault/about-keys-secrets-and-certificates
// for more details.
func OpenSigner(client *keyvault.BaseClient, keyID string, opts *SignerOptions) (*signers.Signer, error) {
	drv, err := openSigner(client, keyID, opts)
	if err != nil {
		return nil, err
	}
	return signers.NewSigner(drv), nil
}

func openSigner(client *keyvault.BaseClient, keyID string, opts *SignerOptions) (*signer, error) {
	if opts == nil {
		opts = &SignerOptions{}
	}
	if opts.Algorithm == "" {
		opts.Algorithm = keyvault.RS512
	}
	matches := keyIdRegex.FindStringSubmatch(keyID)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid keyID %q; must match %v %v", keyID, keyIdRegex, matches)
	}
	// matches[0] is the whole keyID, [1] is the keyVaultURI, and [2] is the key or the key/version.
	keyVaultURI := matches[1]
	parts := strings.SplitN(matches[2], "/", 2)
	keyName := parts[0]
	var keyVersion string
	if len(parts) > 1 {
		keyVersion = parts[1]
	}
	return &signer{
		client:      client,
		keyVaultURI: keyVaultURI,
		keyName:     keyName,
		keyVersion:  keyVersion,
		options:     opts,
	}, nil
}

// Sign creates a signature of a digest using the configured key. The sign
// operation supports both asymmetric and symmetric keys.
//
// In the case of asymmetric keys, the digest must be produced with the
// same digest algorithm as specified by the key.
func (k *signer) Sign(ctx context.Context, digest []byte) (signature []byte, err error) {
	b64digest := base64.RawURLEncoding.EncodeToString(digest)
	result, err := k.client.Sign(ctx, k.keyVaultURI, k.keyName, k.keyVersion, keyvault.KeySignParameters{
		Algorithm: k.options.Algorithm,
		Value:     &b64digest,
	})
	if err != nil {
		return nil, err
	}

	sig, err := base64.RawURLEncoding.DecodeString(*result.Result)
	if err != nil {
		return nil, err
	}

	return sig, nil
}

// Verify verifies a signature using the configured key. The verify
// operation supports both symmetric keys and asymmetric keys.
//
// In case of asymmetric keys public portion of the key is used to verify
// the signature.  Also, the digest must be produced with the same digest
// algorithm as specified by the key.
func (k *signer) Verify(ctx context.Context, digest []byte, signature []byte) (ok bool, err error) {
	b64digest := base64.RawURLEncoding.EncodeToString(digest)
	b64sig := base64.RawURLEncoding.EncodeToString(signature)
	result, err := k.client.Verify(ctx, k.keyVaultURI, k.keyName, k.keyVersion, keyvault.KeyVerifyParameters{
		Algorithm: k.options.Algorithm,
		Digest:    &b64digest,
		Signature: &b64sig,
	})
	if err != nil {
		return false, err
	}

	return *result.Value, nil
}

// Close implements driver.Signer.Close.
func (k *signer) Close() error { return nil }

// ErrorAs implements driver.Signer.ErrorAs.
func (k *signer) ErrorAs(err error, i interface{}) bool {
	e, ok := err.(autorest.DetailedError)
	if !ok {
		return false
	}
	p, ok := i.(*autorest.DetailedError)
	if !ok {
		return false
	}
	*p = e
	return true
}

// ErrorCode implements driver.ErrorCode.
func (k *signer) ErrorCode(err error) gcerrors.ErrorCode {
	de, ok := err.(autorest.DetailedError)
	if !ok {
		return gcerr.Unknown
	}
	ec, ok := errorCodeMap[de.StatusCode.(int)]
	if !ok {
		return gcerr.Unknown
	}
	return ec
}
