// Copyright 2018 The Go Cloud Authors
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

package localsecrets

import (
	"context"
	"testing"

	"github.com/google/go-cloud/internal/secrets/driver"
	"github.com/google/go-cloud/internal/secrets/drivertest"
)

type harness struct{}

func (h harness) MakeDriver(ctx context.Context) (driver.Crypter, error) {
	return NewKeeper(ByteKey("very secret secret")), nil
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return harness{}, nil
}
