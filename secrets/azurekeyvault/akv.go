// Copyright 2019 The Go Cloud Development Kit Authors
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
//
// Package azurekeyvault provides a secrets implementation backed by Azure KeyVault.
// See https://docs.microsoft.com/en-us/azure/key-vault/key-vault-whatis for more information.
// Use NewKeeper to construct a *secrets.Keeper.
//
// As
//
// azurekeyvault exposes the following type for As:
// - Error: autorest.DetailedError, see https://godoc.org/github.com/Azure/go-autorest/autorest#DetailedError
package azurekeyvault

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/useragent"
	"gocloud.dev/secrets"
)

var (
	// KeyVault URI suffix
	keyVaultEndpointSuffix = "vault.azure.net"
	// Map of HTTP Status Code to go-cloud ErrorCode
	errorCodeMap = map[int]gcerrors.ErrorCode{
		200: gcerrors.OK,
		400: gcerrors.InvalidArgument,
		401: gcerrors.PermissionDenied,
		404: gcerrors.NotFound,
		408: gcerrors.DeadlineExceeded,
		429: gcerrors.ResourceExhausted,
		500: gcerrors.Internal,
		501: gcerrors.Unimplemented,
	}
)

type (
	keeper struct {
		client       *keyvault.BaseClient
		keyVaultName string
		keyName      string
		keyVersion   string
		options      *KeeperOptions
	}

	// KeeperOptions provides configuration options for encryption/decryption operations.
	KeeperOptions struct {
		Algorithm string
	}
)

// Dial gets a new *keyvault.BaseClient, see https://godoc.org/github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault#BaseClient
func Dial() (*keyvault.BaseClient, error) {
	auth, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		return nil, err
	}

	client := keyvault.NewWithoutDefaults()
	client.Authorizer = auth
	client.Sender = autorest.NewClientWithUserAgent(useragent.AzureUserAgentPrefix("secrets"))
	return &client, nil
}

// NewKeeper returns a *secrets.Keeper that uses Azure keyVault.
// List of Parameters:
// - client: *keyvault.BaseClient instance, see https://godoc.org/github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault#BaseClient
// - keyVaultName: string representing the KeyVault name, see https://docs.microsoft.com/en-us/azure/key-vault/common-parameters-and-headers
// - keyName: string representing the keyName, see https://docs.microsoft.com/en-us/rest/api/keyvault/encrypt/encrypt#uri-parameters
// - keyVersion: string representing the keyVersion, see https://docs.microsoft.com/en-us/rest/api/keyvault/encrypt/encrypt#uri-parameters
// - opts: *KeeperOptions with the desired Algorithm to use for operations. See this link for more info: https://docs.microsoft.com/en-us/rest/api/keyvault/encrypt/encrypt#jsonwebkeyencryptionalgorithm
func NewKeeper(client *keyvault.BaseClient, keyVaultName, keyName, keyVersion string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		client:       client,
		keyVaultName: keyVaultName,
		keyName:      keyName,
		keyVersion:   keyVersion,
		options:      opts,
	})
}

// Encrypt encrypts the plaintext into a ciphertext.
func (k *keeper) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	if err := k.validateOptions(); err != nil {
		return nil, err
	}

	b64Text := base64.StdEncoding.EncodeToString(plaintext)
	keyOpsResult, err := k.client.Encrypt(ctx, k.getKeyVaultURI(), k.keyName, k.keyVersion, keyvault.KeyOperationsParameters{
		Algorithm: keyvault.JSONWebKeyEncryptionAlgorithm(k.options.Algorithm),
		Value:     &b64Text,
	})
	if err != nil {
		return nil, err
	}

	return []byte(*keyOpsResult.Result), nil
}

// Decrypt decrypts the ciphertext into a plaintext.
func (k *keeper) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	if err := k.validateOptions(); err != nil {
		return nil, err
	}

	cipherval := string(ciphertext)
	keyOpsResult, err := k.client.Decrypt(ctx, k.getKeyVaultURI(), k.keyName, k.keyVersion, keyvault.KeyOperationsParameters{
		Algorithm: keyvault.JSONWebKeyEncryptionAlgorithm(k.options.Algorithm),
		Value:     &cipherval,
	})
	if err != nil {
		return nil, err
	}

	return base64.StdEncoding.DecodeString(*keyOpsResult.Result)
}

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i interface{}) bool {
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
func (k *keeper) ErrorCode(err error) gcerrors.ErrorCode {
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

func (k *keeper) getKeyVaultURI() string {
	return fmt.Sprintf("https://%s.%s/", k.keyVaultName, keyVaultEndpointSuffix)
}

func (k *keeper) validateOptions() error {
	if k.options != nil && k.options.Algorithm == "" {
		return fmt.Errorf("invalid algorithm, choose from %s", getSupportedAlgorithmsForError())
	}

	return nil
}

func getSupportedAlgorithmsForError() string {
	var algos []string
	for _, a := range keyvault.PossibleJSONWebKeyEncryptionAlgorithmValues() {
		algos = append(algos, string(a))
	}
	return strings.Join(algos, ", ")
}
