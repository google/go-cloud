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

// Package awskmsv2 provides a secrets implementation backed by AWS KMS.
// Use OpenKeeper to construct a *secrets.Keeper.
//
// URLs
//
// For secrets.OpenKeeper, awskmsv2 registers for the scheme "awskmsv2".
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// awskmsv2 exposes the following type for As:
//  - Error: any type supported by the service, including smithy.APIError
package awskmsv2 // import "gocloud.dev/secrets/awskmsv2"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsv1 "github.com/aws/aws-sdk-go/service/kms" // has the error codes
	"github.com/aws/smithy-go"

	"github.com/google/wire"
	gcaws "gocloud.dev/awsv2"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/secrets"
)

func init() {
	secrets.DefaultURLMux().RegisterKeeper(Scheme, new(lazySessionOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	wire.Struct(new(URLOpener), "Config"),
	Dial,
)

// Dial gets an AWS KMS service client.
func Dial(cfg aws.Config) (*kms.Client, error) {
	return kms.NewFromConfig(cfg), nil
}

// lazySessionOpener obtains the AWS session from the environment on the first
// call to OpenKeeperURL.
type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	o.init.Do(func() {
		cfg, err := gcaws.NewDefaultConfig(ctx)
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			Config: cfg,
		}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open keeper %v: %v", u, o.err)
	}
	return o.opener.OpenKeeperURL(ctx, u)
}

// Scheme is the URL scheme awskmsv2 registers its URLOpener under on secrets.DefaultMux.
const Scheme = "awskmsv2"

// URLOpener opens AWS KMS URLs like "awskmsv2://keyID" or "awskmsv2:///keyID".
//
// The URL Host + Path are used as the key ID, which can be in the form of an
// Amazon Resource Name (ARN), alias name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details. Note that ARNs may contain ":" characters, which cannot be
// escaped in the Host part of a URL, so the "awskmsv2:///<ARN>" form should be used.
//
// See gocloud.dev/aws/ConfigFromURLParams for supported query parameters
// for overriding the aws.Session from the URL.
type URLOpener struct {
	// Config must be set to a non-nil value.
	Config aws.Config

	// Options specifies the options to pass to OpenKeeper.
	Options KeeperOptions
}

// OpenKeeperURL opens an AWS KMS Keeper based on u.
func (o *URLOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	cfg, err := gcaws.ConfigFromURLParams(ctx, u.Query())
	if err != nil {
		return nil, fmt.Errorf("open keeper %v: %v", u, err)
	}
	// A leading "/" means the Host was empty; trim the slash.
	// This is so that awskmsv2:///foo:bar results in "foo:bar" instead of
	// "/foo:bar".
	keyID := strings.TrimPrefix(path.Join(u.Host, u.Path), "/")
	client := kms.NewFromConfig(cfg)
	return OpenKeeper(client, keyID, &o.Options), nil
}

// OpenKeeper returns a *secrets.Keeper that uses AWS KMS.
// The key ID can be in the form of an Amazon Resource Name (ARN), alias
// name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details.
// See the package documentation for an example.
func OpenKeeper(client *kms.Client, keyID string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		keyID:  keyID,
		client: client,
	})
}

type keeper struct {
	keyID  string
	client *kms.Client
}

// Decrypt decrypts the ciphertext into a plaintext.
func (k *keeper) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	result, err := k.client.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		return nil, err
	}
	return result.Plaintext, nil
}

// Encrypt encrypts the plaintext into a ciphertext.
func (k *keeper) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	result, err := k.client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(k.keyID),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}
	return result.CiphertextBlob, nil
}

// Close implements driver.Keeper.Close.
func (k *keeper) Close() error { return nil }

// ErrorAs implements driver.Keeper.ErrorAs.
func (k *keeper) ErrorAs(err error, i interface{}) bool {
	return errors.As(err, i)
}

// ErrorCode implements driver.ErrorCode.
func (k *keeper) ErrorCode(err error) gcerrors.ErrorCode {
	var ae smithy.APIError
	if !errors.As(err, &ae) {
		return gcerr.Unknown
	}
	ec, ok := errorCodeMap[ae.ErrorCode()]
	if !ok {
		return gcerr.Unknown
	}
	return ec
}

var errorCodeMap = map[string]gcerrors.ErrorCode{
	kmsv1.ErrCodeNotFoundException:          gcerrors.NotFound,
	kmsv1.ErrCodeInvalidCiphertextException: gcerrors.InvalidArgument,
	kmsv1.ErrCodeInvalidKeyUsageException:   gcerrors.InvalidArgument,
	kmsv1.ErrCodeInternalException:          gcerrors.Internal,
	kmsv1.ErrCodeInvalidStateException:      gcerrors.FailedPrecondition,
	kmsv1.ErrCodeDisabledException:          gcerrors.PermissionDenied,
	kmsv1.ErrCodeInvalidGrantTokenException: gcerrors.PermissionDenied,
	kmsv1.ErrCodeKeyUnavailableException:    gcerrors.ResourceExhausted,
	kmsv1.ErrCodeDependencyTimeoutException: gcerrors.DeadlineExceeded,
}

// KeeperOptions controls Keeper behaviors.
// It is provided for future extensibility.
type KeeperOptions struct{}
