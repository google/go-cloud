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

package secrets

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/testing/octest"
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

func (k *erroringKeeper) Close() error                       { return errFake }
func (k *erroringKeeper) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }

func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	k := NewKeeper(&erroringKeeper{})

	// verifyWrap ensures that err is wrapped exactly once.
	verifyWrap := func(description string, err error) {
		if err == nil {
			t.Errorf("%s: got nil error, wanted non-nil", description)
		} else if unwrapped, ok := err.(*gcerr.Error); !ok {
			t.Errorf("%s: not wrapped: %v", description, err)
		} else if du, ok := unwrapped.Unwrap().(*gcerr.Error); ok {
			t.Errorf("%s: double wrapped: %v", description, du)
		}
		if s := err.Error(); !strings.HasPrefix(s, "secrets ") {
			t.Errorf("%s: Error() for wrapped error doesn't start with secrets: prefix: %s", description, s)
		}
	}

	_, err := k.Decrypt(ctx, nil)
	verifyWrap("Decrypt", err)

	_, err = k.Encrypt(ctx, nil)
	verifyWrap("Encrypt", err)

	err = k.Close()
	verifyWrap("Close", err)
}

// TestKeeperIsClosed tests that Keeper functions return an error when the
// Keeper is closed.
func TestKeeperIsClosed(t *testing.T) {
	ctx := context.Background()
	k := NewKeeper(&erroringKeeper{})
	k.Close()

	if _, err := k.Decrypt(ctx, nil); err != errClosed {
		t.Error(err)
	}
	if _, err := k.Encrypt(ctx, nil); err != errClosed {
		t.Error(err)
	}
	if err := k.Close(); err != errClosed {
		t.Error(err)
	}
}

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(OpenCensusViews)
	defer te.Unregister()

	k := NewKeeper(&erroringKeeper{})
	defer k.Close()
	k.Encrypt(ctx, nil)
	k.Decrypt(ctx, nil)
	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/secrets", "gocloud.dev/secrets", []octest.Call{
		{Method: "Encrypt", Code: gcerrors.Internal},
		{Method: "Decrypt", Code: gcerrors.Internal},
	})
	if diff != "" {
		t.Error(diff)
	}
}

var (
	testOpenOnce sync.Once
	testOpenGot  *url.URL
)

func TestURLMux(t *testing.T) {
	ctx := context.Background()

	mux := new(URLMux)
	fake := &fakeOpener{}
	mux.RegisterKeeper("foo", fake)
	mux.RegisterKeeper("err", fake)

	if diff := cmp.Diff(mux.KeeperSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidKeeperScheme("foo") || !mux.ValidKeeperScheme("err") {
		t.Errorf("ValidKeeperScheme didn't return true for valid scheme")
	}
	if mux.ValidKeeperScheme("foo2") || mux.ValidKeeperScheme("http") {
		t.Errorf("ValidKeeperScheme didn't return false for invalid scheme")
	}

	for _, tc := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "empty URL",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     ":foo",
			wantErr: true,
		},
		{
			name:    "invalid URL no scheme",
			url:     "foo",
			wantErr: true,
		},
		{
			name:    "unregistered scheme",
			url:     "bar://mykeeper",
			wantErr: true,
		},
		{
			name:    "func returns error",
			url:     "err://mykeeper",
			wantErr: true,
		},
		{
			name: "no query options",
			url:  "foo://mykeeper",
		},
		{
			name: "empty query options",
			url:  "foo://mykeeper?",
		},
		{
			name: "query options",
			url:  "foo://mykeeper?aAa=bBb&cCc=dDd",
		},
		{
			name: "multiple query options",
			url:  "foo://mykeeper?x=a&x=b&x=c",
		},
		{
			name: "fancy keeper name",
			url:  "foo:///foo/bar/baz",
		},
		{
			name: "using api scheme prefix",
			url:  "secrets+foo://mykeeper",
		},
		{
			name: "using api+type scheme prefix",
			url:  "secrets+keeper+foo://mykeeper",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keeper, gotErr := mux.OpenKeeper(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			defer keeper.Close()
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenKeeperURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			keeper, gotErr = mux.OpenKeeperURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v, want nil", gotErr)
			}
			defer keeper.Close()
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
	}
}

type fakeOpener struct {
	u *url.URL // last url passed to OpenKeeperURL
}

func (o *fakeOpener) OpenKeeperURL(ctx context.Context, u *url.URL) (*Keeper, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return NewKeeper(&erroringKeeper{}), nil
}
