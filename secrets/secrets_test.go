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
}

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(OpenCensusViews)
	defer te.Unregister()

	k := NewKeeper(&erroringKeeper{})
	k.Encrypt(ctx, nil)
	k.Decrypt(ctx, nil)
	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/secrets", "gocloud.dev/secrets", []octest.Call{
		{"Encrypt", gcerrors.Internal},
		{"Decrypt", gcerrors.Internal},
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
			_, gotErr := mux.OpenKeeper(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenKeeperURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr = mux.OpenKeeperURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v, want nil", gotErr)
			}
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
	return nil, nil
}
