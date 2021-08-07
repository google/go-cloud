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
// Use OpenKeeper to construct a *secrets.Keeper, or OpenKeeperV2 to
// use AWS SDK V2.
//
// URLs
//
// For secrets.OpenKeeper, awskms registers for the scheme "awskms".
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// Use "awssdk=v1" or "awssdk=v2" to force a specific AWS SDK version.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// awskms exposes the following type for As:
//  - Error: (V1) awserr.Error, (V2) any error type returned by the service, notably smithy.APIError
package awskms // import "gocloud.dev/secrets/awskms"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	kmsv2 "github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/smithy-go"
	"github.com/google/wire"
	gcaws "gocloud.dev/aws"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/secrets"
)

func init() {
	secrets.DefaultURLMux().RegisterKeeper(Scheme, new(lazySessionOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	wire.Struct(new(URLOpener), "ConfigProvider"),
	Dial,
	DialV2,
)

// Dial gets an AWS KMS service client.
func Dial(p client.ConfigProvider) (*kms.KMS, error) {
	if p == nil {
		return nil, errors.New("getting KMS service: no AWS session provided")
	}
	return kms.New(p), nil
}

// DialV2 gets an AWS KMS service client using the AWS SDK V2.
func DialV2(cfg awsv2.Config) (*kmsv2.Client, error) {
	return kmsv2.NewFromConfig(cfg), nil
}

// lazySessionOpener obtains the AWS session from the environment on the first
// call to OpenKeeperURL.
type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	if gcaws.UseV2(u.Query()) {
		opener := &URLOpener{UseV2: true}
		return opener.OpenKeeperURL(ctx, u)
	}
	o.init.Do(func() {
		sess, err := gcaws.NewDefaultSession()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			UseV2:          false,
			ConfigProvider: sess,
		}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open keeper %v: %v", u, o.err)
	}
	return o.opener.OpenKeeperURL(ctx, u)
}

// Scheme is the URL scheme awskms registers its URLOpener under on secrets.DefaultMux.
const Scheme = "awskms"

// URLOpener opens AWS KMS URLs like "awskms://keyID" or "awskms:///keyID".
//
// The URL Host + Path are used as the key ID, which can be in the form of an
// Amazon Resource Name (ARN), alias name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details. Note that ARNs may contain ":" characters, which cannot be
// escaped in the Host part of a URL, so the "awskms:///<ARN>" form should be used.
//
// Use "awssdk=v1" to force using AWS SDK v1, "awssdk=v2" to force using AWS SDK v2,
// or anything else to accept the default.
//
// For V1, see gocloud.dev/aws/ConfigFromURLParams for supported query parameters
// for overriding the aws.Session from the URL.
// For V2, see gocloud.dev/aws/V2ConfigFromURLParams.
type URLOpener struct {
	// UseV2 indicates whether the AWS SDK V2 should be used.
	UseV2 bool

	// ConfigProvider must be set to a non-nil value if UseV2 is false.
	ConfigProvider client.ConfigProvider

	// Options specifies the options to pass to OpenKeeper.
	Options KeeperOptions
}

// OpenKeeperURL opens an AWS KMS Keeper based on u.
func (o *URLOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*secrets.Keeper, error) {
	// A leading "/" means the Host was empty; trim the slash.
	// This is so that awskms:///foo:bar results in "foo:bar" instead of
	// "/foo:bar".
	keyID := strings.TrimPrefix(path.Join(u.Host, u.Path), "/")

	if o.UseV2 {
		cfg, err := gcaws.V2ConfigFromURLParams(ctx, u.Query())
		if err != nil {
			return nil, fmt.Errorf("open keeper %v: %v", u, err)
		}
		clientV2, err := DialV2(cfg)
		if err != nil {
			return nil, err
		}
		return OpenKeeperV2(clientV2, keyID, &o.Options), nil
	}
	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(u.Query())
	if err != nil {
		return nil, fmt.Errorf("open keeper %v: %v", u, err)
	}
	configProvider.Configs = append(configProvider.Configs, overrideCfg)
	client, err := Dial(configProvider)
	if err != nil {
		return nil, err
	}
	return OpenKeeper(client, keyID, &o.Options), nil
}

// OpenKeeper returns a *secrets.Keeper that uses AWS KMS.
// The key ID can be in the form of an Amazon Resource Name (ARN), alias
// name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details.
// See the package documentation for an example.
func OpenKeeper(client *kms.KMS, keyID string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		useV2:  false,
		keyID:  keyID,
		client: client,
	})
}

// OpenKeeperV2 returns a *secrets.Keeper that uses AWS KMS, using SDK v2.
// The key ID can be in the form of an Amazon Resource Name (ARN), alias
// name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details.
// See the package documentation for an example.
func OpenKeeperV2(client *kmsv2.Client, keyID string, opts *KeeperOptions) *secrets.Keeper {
	return secrets.NewKeeper(&keeper{
		useV2:    true,
		keyID:    keyID,
		clientV2: client,
	})
}

type keeper struct {
	useV2    bool
	keyID    string
	client   *kms.KMS
	clientV2 *kmsv2.Client
}

// Decrypt decrypts the ciphertext into a plaintext.
func (k *keeper) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	if k.useV2 {
		result, err := k.clientV2.Decrypt(ctx, &kmsv2.DecryptInput{
			CiphertextBlob: ciphertext,
		})
		if err != nil {
			return nil, err
		}
		return result.Plaintext, nil
	}
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
	if k.useV2 {
		result, err := k.clientV2.Encrypt(ctx, &kmsv2.EncryptInput{
			KeyId:     aws.String(k.keyID),
			Plaintext: plaintext,
		})
		if err != nil {
			return nil, err
		}
		return result.CiphertextBlob, nil
	}
	result, err := k.client.Encrypt(&kms.EncryptInput{
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
	if k.useV2 {
		return errors.As(err, i)
	}
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
func (k *keeper) ErrorCode(err error) gcerrors.ErrorCode {
	var code string
	if k.useV2 {
		var ae smithy.APIError
		if !errors.As(err, &ae) {
			return gcerr.Unknown
		}
		code = ae.ErrorCode()
	} else {
		ae, ok := err.(awserr.Error)
		if !ok {
			return gcerr.Unknown
		}
		code = ae.Code()
	}
	ec, ok := errorCodeMap[code]
	if !ok {
		return gcerr.Unknown
	}
	return ec
}

var errorCodeMap = map[string]gcerrors.ErrorCode{
	kms.ErrCodeNotFoundException:          gcerrors.NotFound,
	kms.ErrCodeInvalidCiphertextException: gcerrors.InvalidArgument,
	kms.ErrCodeInvalidKeyUsageException:   gcerrors.InvalidArgument,
	kms.ErrCodeInternalException:          gcerrors.Internal,
	kms.ErrCodeInvalidStateException:      gcerrors.FailedPrecondition,
	kms.ErrCodeDisabledException:          gcerrors.PermissionDenied,
	kms.ErrCodeInvalidGrantTokenException: gcerrors.PermissionDenied,
	kms.ErrCodeKeyUnavailableException:    gcerrors.ResourceExhausted,
	kms.ErrCodeDependencyTimeoutException: gcerrors.DeadlineExceeded,
}

// KeeperOptions controls Keeper behaviors.
// It is provided for future extensibility.
type KeeperOptions struct{}
