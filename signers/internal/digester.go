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

package internal

import (
	"crypto"
	"fmt"
	"strings"

	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
)

type Digester interface {
	HashFunc() crypto.Hash
	Digest(plaintext []byte) []byte
	Wrap(digest []byte) *kmspb.Digest
}

type SHA256 struct{}

func (s *SHA256) HashFunc() crypto.Hash {
	return crypto.SHA256
}

func (s *SHA256) Digest(plaintext []byte) []byte {
	digester := crypto.SHA256.New()
	_, _ = fmt.Fprint(digester, plaintext)
	return digester.Sum(nil)
}

func (s *SHA256) Wrap(digest []byte) *kmspb.Digest {
	return &kmspb.Digest{
		Digest: &kmspb.Digest_Sha256{
			Sha256: digest,
		},
	}
}

type SHA384 struct{}

func (s *SHA384) HashFunc() crypto.Hash {
	return crypto.SHA384
}

func (s *SHA384) Digest(plaintext []byte) []byte {
	digester := crypto.SHA384.New()
	_, _ = fmt.Fprint(digester, plaintext)
	return digester.Sum(nil)
}

func (s *SHA384) Wrap(digest []byte) *kmspb.Digest {
	return &kmspb.Digest{
		Digest: &kmspb.Digest_Sha384{
			Sha384: digest,
		},
	}
}

type SHA512 struct{}

func (s *SHA512) HashFunc() crypto.Hash {
	return crypto.SHA512
}

func (s *SHA512) Digest(plaintext []byte) []byte {
	digester := crypto.SHA512.New()
	_, _ = fmt.Fprint(digester, plaintext)
	return digester.Sum(nil)
}

func (s *SHA512) Wrap(digest []byte) *kmspb.Digest {
	return &kmspb.Digest{
		Digest: &kmspb.Digest_Sha512{
			Sha512: digest,
		},
	}
}

func DigesterFromString(name string) (Digester, error) {
	switch strings.ToLower(name) {
	case "sha256":
		return &SHA256{}, nil
	case "sha384":
		return &SHA384{}, nil
	case "sha512":
		return &SHA512{}, nil
	}
	return nil, fmt.Errorf("unsupported hash type: %s", name)
}
