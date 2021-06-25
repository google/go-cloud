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
//  limitations under the License.
//

package signers

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/testing/octest"
	"gocloud.dev/signers/driver"
)

var errFake = errors.New("fake")

type erroringSigner struct {
	driver.Signer
}

func (k *erroringSigner) Sign(_ context.Context, _ []byte) ([]byte, error) {
	return nil, errFake
}

func (k *erroringSigner) Verify(_ context.Context, _ []byte, _ []byte) (ok bool, err error) {
	return false, errFake
}

func (k *erroringSigner) Close() error                       { return errFake }
func (k *erroringSigner) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }

func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	k := NewSigner(&erroringSigner{})

	// verifyWrap ensures that err is wrapped exactly once.
	verifyWrap := func(description string, err error) {
		if err == nil {
			t.Errorf("%s: got nil error, wanted non-nil", description)
		} else if unwrapped, ok := err.(*gcerr.Error); !ok {
			t.Errorf("%s: not wrapped: %v", description, err)
		} else if du, ok := unwrapped.Unwrap().(*gcerr.Error); ok {
			t.Errorf("%s: double wrapped: %v", description, du)
		}
		if s := err.Error(); !strings.HasPrefix(s, "signers ") {
			t.Errorf("%s: Error() for wrapped error doesn't start with signers: prefix: %s", description, s)
		}
	}

	_, err := k.Sign(ctx, nil)
	verifyWrap("Sign", err)

	err = k.Close()
	verifyWrap("Close", err)
}

// TestSignerIsClosed tests that Signer functions return an error when the
// Signer is closed.
func TestSignerIsClosed(t *testing.T) {
	ctx := context.Background()
	k := NewSigner(&erroringSigner{})
	k.Close()

	if _, err := k.Sign(ctx, nil); err != errClosed {
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

	k := NewSigner(&erroringSigner{})
	defer k.Close()
	_, _ = k.Sign(ctx, nil)
	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/signers", "gocloud.dev/signers", []octest.Call{
		{Method: "Sign", Code: gcerrors.Internal},
	})
	if diff != "" {
		t.Error(diff)
	}
}

func TestURLMux(t *testing.T) {
	ctx := context.Background()

	mux := new(URLMux)
	fake := &fakeOpener{}
	mux.RegisterSigner("foo", fake)
	mux.RegisterSigner("err", fake)

	if diff := cmp.Diff(mux.SignerSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidSignerScheme("foo") || !mux.ValidSignerScheme("err") {
		t.Errorf("ValidSignerScheme didn't return true for valid scheme")
	}
	if mux.ValidSignerScheme("foo2") || mux.ValidSignerScheme("http") {
		t.Errorf("ValidSignerScheme didn't return false for invalid scheme")
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
			url:     "bar://mysigner",
			wantErr: true,
		},
		{
			name:    "func returns error",
			url:     "err://mysigner",
			wantErr: true,
		},
		{
			name: "no query options",
			url:  "foo://mysigner",
		},
		{
			name: "empty query options",
			url:  "foo://mysigner?",
		},
		{
			name: "query options",
			url:  "foo://mysigner?aAa=bBb&cCc=dDd",
		},
		{
			name: "multiple query options",
			url:  "foo://mysigner?x=a&x=b&x=c",
		},
		{
			name: "fancy signer name",
			url:  "foo:///foo/bar/baz",
		},
		{
			name: "using api scheme prefix",
			url:  "signers+foo://mysigner",
		},
		{
			name: "using api+type scheme prefix",
			url:  "signers+signer+foo://mysigner",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signer, gotErr := mux.OpenSigner(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			defer signer.Close()
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenSignerURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			signer, gotErr = mux.OpenSignerURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v, want nil", gotErr)
			}
			defer signer.Close()
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
	}
}

type fakeOpener struct {
	u *url.URL // last url passed to OpenSignerURL
}

func (o *fakeOpener) OpenSignerURL(_ context.Context, u *url.URL) (*Signer, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return NewSigner(&erroringSigner{}), nil
}
