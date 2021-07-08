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

// Package awssig provides a signers implementation backed by AWS KMS.
// Use OpenSigner to construct a *signers.Signer.
//
// URLs
//
// For signers.OpenSigner, awssig registers for the scheme "awssig".
// The default URL opener will use an AWS session with the default credentials
// and configuration; see https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for more details.
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// As
//
// awssig exposes the following type for As:
//  - Error: awserr.Error
package awssig // import "gocloud.dev/signers/awssig"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/google/wire"

	gcaws "gocloud.dev/aws"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/signers"
)

func init() {
	signers.DefaultURLMux().RegisterSigner(Scheme, new(lazySessionOpener))
}

// Set holds Wire providers for this package.
var Set = wire.NewSet(
	wire.Struct(new(URLOpener), "ConfigProvider"),
	Dial,
)

// Dial gets an AWS KMS service client.
func Dial(p client.ConfigProvider) (*kms.KMS, error) {
	if p == nil {
		return nil, errors.New("getting KMS service: no AWS session provided")
	}
	return kms.New(p), nil
}

// lazySessionOpener obtains the AWS session from the environment on the first
// call to OpenSignerURL.
type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) OpenSignerURL(ctx context.Context, u *url.URL) (*signers.Signer, error) {
	o.init.Do(func() {
		sess, err := gcaws.NewDefaultSession()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			ConfigProvider: sess,
		}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open signer %v: %v", u, o.err)
	}
	return o.opener.OpenSignerURL(ctx, u)
}

// Scheme is the URL scheme awssig registers its URLOpener under on signers.DefaultMux.
const Scheme = "awssig"

// URLOpener opens AWS KMS URLs like
//
//    awssig://[ALGORITHM]/keyID
//
// or
//
//    awssig:///keyID
//
// The URL Host is used to specify the signing algorithm to use.  If no
// algorithm is specified, then the first signing algorithm available to the
// signing key is used; the set of available signing algorithms is obtained by
// obtaining the public key from AWS KMS, i.e. a call to AWS.
// The URL Path is used as the key ID, which can be in the form of an
// Amazon Resource Name (ARN), alias name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details.
//
// See gocloud.dev/aws/ConfigFromURLParams for supported query parameters
// for overriding the aws.Session from the URL.
type URLOpener struct {
	// ConfigProvider must be set to a non-nil value.
	ConfigProvider client.ConfigProvider

	// Options specifies the options to pass to OpenSigner.
	Options SignerOptions
}

// OpenSignerURL opens an AWS KMS Signer based on u.
func (o *URLOpener) OpenSignerURL(_ context.Context, u *url.URL) (*signers.Signer, error) {
	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(u.Query())
	if err != nil {
		return nil, fmt.Errorf("open signer %v: %v", u, err)
	}
	configProvider.Configs = append(configProvider.Configs, overrideCfg)
	c, err := Dial(configProvider)
	if err != nil {
		return nil, err
	}
	algorithm := u.Host
	keyID := u.Path[1:]
	return OpenSigner(c, algorithm, keyID, &o.Options), nil
}

// OpenSigner returns a *signers.Signer that uses AWS KMS.
// The key ID can be in the form of an Amazon Resource Name (ARN), alias
// name, or alias ARN. See
// https://docs.aws.amazon.com/kms/latest/developerguide/viewing-keys.html#find-cmk-id-arn
// for more details.
// See the package documentation for an example.
func OpenSigner(client *kms.KMS, algorithm string, keyID string, opts *SignerOptions) *signers.Signer {
	return signers.NewSigner(&signer{
		algorithm: algorithm,
		keyID:     keyID,
		client:    client,
	})
}

type signer struct {
	algorithm string
	keyID     string
	client    *kms.KMS
}

// Sign creates a signature of a digest using the configured key. The sign
// operation supports both asymmetric and symmetric keys.
//
// In the case of asymmetric keys, the digest must be produced with the
// same digest algorithm as specified by the key.
func (k *signer) Sign(ctx context.Context, digest []byte) ([]byte, error) {
	algorithm, err := k.signingAlgorithm(k.algorithm)
	if err != nil {
		return nil, err
	}
	result, err := k.client.Sign(&kms.SignInput{
		KeyId:            aws.String(k.keyID),
		Message:          digest,
		SigningAlgorithm: aws.String(algorithm),
	})
	if err != nil {
		return nil, err
	}
	return result.Signature, nil
}

// Verify verifies a signature using the configured key. The verify
// operation supports both symmetric keys and asymmetric keys.
//
// In case of asymmetric keys public portion of the key is used to verify
// the signature.  Also, the digest must be produced with the same digest
// algorithm as specified by the key.
func (k *signer) Verify(ctx context.Context, digest []byte, signature []byte) (bool, error) {
	algorithm, err := k.signingAlgorithm(k.algorithm)
	if err != nil {
		return false, err
	}
	_, err = k.client.Verify(&kms.VerifyInput{
		KeyId:            aws.String(k.keyID),
		Message:          digest,
		Signature:        signature,
		SigningAlgorithm: aws.String(algorithm),
	})
	return err == nil, err
}

func (k *signer) signingAlgorithm(algorithm string) (string, error) {
	if algorithm != "" {
		return algorithm, nil
	}
	resp, err := k.client.GetPublicKey(&kms.GetPublicKeyInput{
		KeyId: aws.String(k.keyID),
	})
	if err != nil {
		return "", err
	}

	var algorithms = make([]string, len(resp.SigningAlgorithms))
	for i, a := range resp.SigningAlgorithms {
		algorithms[i] = *a
	}
	sort.Strings(algorithms)

	return algorithms[0], nil
}

// Close implements driver.Signer.Close.
func (k *signer) Close() error { return nil }

// ErrorAs implements driver.Signer.ErrorAs.
func (k *signer) ErrorAs(err error, i interface{}) bool {
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
func (k *signer) ErrorCode(err error) gcerrors.ErrorCode {
	ae, ok := err.(awserr.Error)
	if !ok {
		return gcerr.Unknown
	}
	ec, ok := errorCodeMap[ae.Code()]
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

// SignerOptions controls Signer behaviors.
// It is provided for future extensibility.
type SignerOptions struct{}
