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
	"bytes"
	"crypto"
	"fmt"
	"testing"
)

var tests = []struct {
	name string
	d    Digester
	hash crypto.Hash
}{
	{"sha256", &SHA256{}, crypto.SHA256},
	{"sha384", &SHA384{}, crypto.SHA384},
	{"sha512", &SHA512{}, crypto.SHA512},
}

func TestDigest(t *testing.T) {
	const msg = "how now brown cow"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digester := test.hash.New()
			_, _ = fmt.Fprint(digester, []byte(msg))
			dpb := digester.Sum(nil)
			dpb1 := test.d.Digest([]byte(msg))
			if !bytes.Equal(dpb, dpb1) {
				t.Errorf("not equal")
			}
			dpb2 := test.d.Digest([]byte(msg))
			if !bytes.Equal(dpb1, dpb2) {
				t.Errorf("not equal")
			}
		})
	}
}
