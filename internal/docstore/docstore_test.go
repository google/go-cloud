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

package docstore

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore/driver"
)

var (
	testOpenOnce sync.Once
	testOpenGot  *url.URL
)

func TestURLMux(t *testing.T) {
	ctx := context.Background()

	mux := new(URLMux)
	fake := &fakeOpener{}
	mux.RegisterCollection("foo", fake)
	mux.RegisterCollection("err", fake)

	if diff := cmp.Diff(mux.CollectionSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidCollectionScheme("foo") || !mux.ValidCollectionScheme("err") {
		t.Errorf("ValidCollectionScheme didn't return true for valid scheme")
	}
	if mux.ValidCollectionScheme("foo2") || mux.ValidCollectionScheme("http") {
		t.Errorf("ValidCollectionScheme didn't return false for invalid scheme")
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
			url:     "bar://mycollection",
			wantErr: true,
		},
		{
			name:    "func returns error",
			url:     "err://mycollection",
			wantErr: true,
		},
		{
			name: "no query options",
			url:  "foo://mycollection",
		},
		{
			name: "empty query options",
			url:  "foo://mycollection?",
		},
		{
			name: "using api scheme prefix",
			url:  "docstore+foo://bar",
		},
		{
			name: "using api+type scheme prefix",
			url:  "docstore+collection+foo://bar",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := mux.OpenCollection(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenCollectionURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr = mux.OpenCollectionURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v want nil", gotErr)
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
	}
}

type fakeOpener struct {
	u *url.URL // last url passed to OpenCollectionURL
}

func (o *fakeOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*Collection, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return nil, nil
}

func TestToDriverMods(t *testing.T) {
	for _, test := range []struct {
		mods    Mods
		want    []driver.Mod
		wantErr bool
	}{
		{
			Mods{"a": 1, "b": nil},
			[]driver.Mod{{[]string{"a"}, 1}, {[]string{"b"}, nil}},
			false,
		},
		{
			Mods{"a.b": 1, "b.c": nil},
			[]driver.Mod{{[]string{"a", "b"}, 1}, {[]string{"b", "c"}, nil}},
			false,
		},
		// prefixes are not allowed
		{Mods{"a.b.c": 1, "a.b": 2, "a.b+c": 3}, nil, true},
	} {
		got, gotErr := toDriverMods(test.mods)
		if test.wantErr {
			if gotErr == nil {
				t.Errorf("%v: got nil, want error", test.mods)
			}
		} else if !cmp.Equal(got, test.want) {
			t.Errorf("%v: got %v, want %v", test.mods, got, test.want)
		}
	}
}

func TestIsIncNumber(t *testing.T) {
	for _, x := range []interface{}{int(1), 'x', uint(1), byte(1), float32(1), float64(1), time.Duration(1)} {
		if !isIncNumber(x) {
			t.Errorf("%v: got false, want true", x)
		}
	}
	for _, x := range []interface{}{1 + 1i, "3", time.Time{}} {
		if isIncNumber(x) {
			t.Errorf("%v: got true, want false", x)
		}
	}
}
