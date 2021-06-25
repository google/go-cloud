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

package localsigners

import (
	"crypto"
	_ "crypto/md5"
	_ "crypto/sha1"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"math"
	"strings"

	_ "golang.org/x/crypto/blake2b"
	_ "golang.org/x/crypto/blake2s"
	_ "golang.org/x/crypto/md4"
	_ "golang.org/x/crypto/ripemd160"
	_ "golang.org/x/crypto/sha3"
)

var hashes = make(map[string]crypto.Hash, math.MaxUint8)

func init() {
	for hash := crypto.MD4; hash < math.MaxUint8; hash++ {
		if hash.Available() {
			hashes[strings.ToLower(hash.String())] = hash
		}
	}
}

func strToHash(name string) (crypto.Hash, error) {
	hash, ok := hashes[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("invalid hash name: %s", name)
	}
	return hash, nil
}
