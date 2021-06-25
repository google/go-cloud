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

package azuresig

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"

	"gocloud.dev/signers/internal"
)

func algorithmToDigester(algorithm keyvault.JSONWebKeySignatureAlgorithm) (internal.Digester, error) {
	switch algorithm {
	case keyvault.ES256,
		keyvault.ES256K,
		keyvault.PS256,
		keyvault.RS256:
		return &internal.SHA256{}, nil
	case keyvault.ES384,
		keyvault.PS384,
		keyvault.RS384:
		return &internal.SHA384{}, nil
	case keyvault.ES512,
		keyvault.PS512,
		keyvault.RS512:
		return &internal.SHA512{}, nil

	}
	return nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
}
