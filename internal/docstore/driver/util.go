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

package driver

import (
	"math/rand"
	"sync"

	"github.com/google/uuid"
)

// UniqueString generates a string that is unique with high probability.
// Driver implementations can use it to generate keys for Create actions.
func UniqueString() string { return uuid.New().String() }

// Call when running tests that will be replayed.
// Each seed value will result in UniqueString producing the same sequence of values.
func MakeUniqueStringDeterministicForTesting(seed int64) {
	r := &randReader{r: rand.New(rand.NewSource(seed))}
	uuid.SetRand(r)
}

type randReader struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (r *randReader) Read(buf []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.r.Read(buf)
}
