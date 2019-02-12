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

// Package awskms provides a secrets implementation backed by AWS KMS.
// Use NewKeeper to construct a *secrets.Keeper.
//
// As
//
// awskms exposes the following type for As:
//  - Error: awserr.Error
package awskms // import "gocloud.dev/secrets/awskms"

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/kms"
	"gocloud.dev/gcerrors"
	"gocloud.dev/secrets"
)

// NewKeeper returns a *secrets.Keeper that uses AWS KMS.
// The keyID can be in the form of an Amazon Resource Name (ARN), alias
// name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details.
// See the package documentation for an example.
func NewKeeper(client *kms.KMS, keyID string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		keyID:  keyID,
		client: client,
	})
}

// Dial gets a AWS KMS service client.
func Dial(p client.ConfigProvider) (*kms.KMS, error) {
	if p == nil {
		return nil, errors.New("getting KMS service: no AWS session provided")
	}
	return kms.New(p), nil
}

type keeper struct {
	keyID  string
	client *kms.KMS
}

// Decrypt decrypts the ciphertext into a plaintext.
func (k *keeper) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	result, err := k.client.Decrypt(&kms.DecryptInput{
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		return nil, err
	}
	return result.Plaintext, nil
}

// Encrypt encrypts the plaintext into a ciphertext.
func (k *keeper) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	result, err := k.client.Encrypt(&kms.EncryptInput{
		KeyId:     aws.String(k.keyID),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}
	return result.CiphertextBlob, nil
}

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i interface{}) bool {
	e, ok := err.(awserr.Error)
	if !ok {
		return false
	}
	p, ok := i.(*awserr.Error)
	if !ok {
		return false
	}
	*p = e
	return true
}

// ErrorCode implements driver.ErrorCode.
func (k *keeper) ErrorCode(error) gcerrors.ErrorCode {
	// TODO(shantuo): try to classify aws error codes
	return gcerrors.Unknown
}

// KeeperOptions controls Keeper behaviors.
// It is provided for future extensibility.
type KeeperOptions struct{}
