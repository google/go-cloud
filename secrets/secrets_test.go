// Copyright 2019 The Go Cloud Authors
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

package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gocloud.dev/secrets/driver"
)

var errFake = errors.New("fake")

type erroringKeeper struct {
	driver.Keeper
}

func (k *erroringKeeper) Decrypt(ctx context.Context, b []byte) ([]byte, error) {
	return nil, errFake
}

func (k *erroringKeeper) Encrypt(ctx context.Context, b []byte) ([]byte, error) {
	return nil, errFake
}

func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	k := NewKeeper(&erroringKeeper{})

	// verifyWrap ensures that err is wrapped exactly once.
	verifyWrap := func(description string, err error) {
		if err == nil {
			t.Errorf("%s: got nil error, wanted non-nil", description)
		} else if unwrapped, ok := err.(*wrappedError); !ok {
			t.Errorf("%s: not wrapped: %v", description, err)
		} else if du, ok := unwrapped.err.(*wrappedError); ok {
			t.Errorf("%s: double wrapped: %v", description, du)
		}
		if s := err.Error(); !strings.HasPrefix(s, "secrets: ") {
			t.Errorf("%s: Error() for wrapped error doesn't start with secrets: prefix: %s", description, s)
		}
	}

	_, err := k.Decrypt(ctx, nil)
	verifyWrap("Decrypt", err)

	_, err = k.Encrypt(ctx, nil)
	verifyWrap("Encrypt", err)
}
