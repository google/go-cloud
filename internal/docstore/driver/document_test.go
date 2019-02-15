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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type S struct {
	I int
	M map[string]interface{}
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
