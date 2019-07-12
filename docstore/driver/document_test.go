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
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
)

type S struct {
	I int
	M map[string]interface{}
}

func TestNewDocument(t *testing.T) {
	for _, test := range []struct {
		in      interface{}
		wantErr bool
		wantMap bool
	}{
		{in: nil, wantErr: true},
		{in: map[string]interface{}{}, wantMap: true},
		{in: map[string]interface{}(nil), wantErr: true},
		{in: S{}, wantErr: true},
		{in: &S{}, wantMap: false},
		{in: (*S)(nil), wantErr: true},
		{in: map[string]bool{}, wantErr: true},
	} {
		got, err := NewDocument(test.in)
		if err != nil {
			if !test.wantErr {
				t.Errorf("%#v: got %v, did not want error", test.in, err)
			}
			if c := gcerrors.Code(err); c != gcerrors.InvalidArgument {
				t.Errorf("%#v: got error code %s, want InvalidArgument", test.in, c)
			}
			continue
		}
		if test.wantErr {
			t.Errorf("%#v: got nil, want error", test.in)
			continue
		}
		if g := (got.m != nil); g != test.wantMap {
			t.Errorf("%#v: got map: %t, want map: %t", test.in, g, test.wantMap)
		}
	}
}

func TestGet(t *testing.T) {
	in := map[string]interface{}{
		"S": &S{
			I: 2,
			M: map[string]interface{}{
				"J": 3,
				"T": &S{I: 4},
			},
		},
	}
	doc, err := NewDocument(in)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		fp   string
		want interface{}
	}{
		{"S.I", 2},
		{"S.i", 2},
		{"S.M.J", 3},
		{"S.m.J", 3},
		{"S.M.T.I", 4},
		{"S.m.T.i", 4},
	} {
		fp := strings.Split(test.fp, ".")
		got, err := doc.Get(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", got, test.fp, test.want)
		}
	}
}

func TestSet(t *testing.T) {
	in := map[string]interface{}{
		"S": &S{
			I: 2,
			M: map[string]interface{}{
				"J": 3,
				"T": &S{I: 4},
			},
		},
	}
	doc, err := NewDocument(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		fp  string
		val interface{}
	}{
		{"S.I", -1},
		{"S.i", -2},
		{"S.M.J", -3},
		{"S.m.J", -4},
		{"S.M.T.I", -5},
		{"S.m.T.i", -6},
		{"new.field", -7},
	} {
		fp := strings.Split(test.fp, ".")
		if err := doc.Set(fp, test.val); err != nil {
			t.Fatalf("%q: %v", test.fp, err)
		}
		got, err := doc.Get(fp)
		if err != nil {
			t.Fatalf("%s: %v", test.fp, err)
		}
		if !cmp.Equal(got, test.val) {
			t.Errorf("got %v, want %v", got, test.val)
		}
	}
}

func TestGetField(t *testing.T) {
	type S struct {
		A int         `docstore:"a"`
		B interface{} `docstore:"b"`
	}

	want := 1
	for _, in := range []interface{}{
		map[string]interface{}{"a": want, "b": nil},
		&S{A: want, B: nil},
	} {
		t.Run(fmt.Sprint(in), func(t *testing.T) {
			doc, err := NewDocument(in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := doc.GetField("a")
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("got %v, want %v", got, want)
			}

			got, err = doc.GetField("b")
			if err != nil {
				t.Fatal(err)
			}
			if got != nil {
				t.Errorf("got %v, want nil", got)
			}

			_, err = doc.GetField("c")
			if gcerrors.Code(err) != gcerrors.NotFound {
				t.Fatalf("got %v, want NotFound", err)
			}
		})
	}
}

func TestFieldNames(t *testing.T) {
	type E struct {
		C int
	}
	type S struct {
		A int `docstore:"a"`
		B int
		E
	}

	for _, test := range []struct {
		in   interface{}
		want []string
	}{
		{
			map[string]interface{}{"a": 1, "b": map[string]interface{}{"c": 2}},
			[]string{"a", "b"},
		},
		{
			&S{},
			[]string{"B", "C", "a"},
		},
	} {
		doc, err := NewDocument(test.in)
		if err != nil {
			t.Fatal(err)
		}
		got := doc.FieldNames()
		sort.Strings(got)
		if !cmp.Equal(got, test.want) {
			t.Errorf("%v: got %v, want %v", test.in, got, test.want)
		}
	}
}

func TestHasField(t *testing.T) {
	type withRev struct {
		Rev interface{}
	}
	type withoutRev struct {
		W withRev
	}

	for _, tc := range []struct {
		in   interface{}
		want bool
	}{
		{
			in:   &withRev{},
			want: true,
		},
		{
			in:   &withoutRev{},
			want: false,
		},
		{
			in:   map[string]interface{}{"Rev": nil},
			want: true,
		},
		{
			in:   map[string]interface{}{},
			want: false,
		},
	} {
		doc, err := NewDocument(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		on := doc.HasField("Rev")
		if on != tc.want {
			t.Errorf("%v: got %v want %v", tc.in, on, tc.want)
		}
	}
}

func TestHasFieldFold(t *testing.T) {
	type withRev struct {
		Rev interface{}
	}
	type withoutRev struct {
		W withRev
	}

	for _, tc := range []struct {
		in   interface{}
		want bool
	}{
		{
			in:   &withRev{},
			want: true,
		},
		{
			in:   &withoutRev{},
			want: false,
		},
	} {
		doc, err := NewDocument(tc.in)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"Rev", "rev", "REV"} {
			on := doc.HasFieldFold(f)
			if on != tc.want {
				t.Errorf("%v: got %v want %v", tc.in, on, tc.want)
			}
		}
	}
}
