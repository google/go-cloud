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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSplitActions(t *testing.T) {
	in := []*Action{
		{Kind: Get},
		{Kind: Get},
		{Kind: Put},
		{Kind: Update},
		{Kind: Get},
		{Kind: Create},
	}

	for _, test := range []struct {
		desc  string
		split func(a, b *Action) bool
		want  [][]*Action
	}{
		{
			"always false",
			func(a, b *Action) bool { return false },
			[][]*Action{in},
		},
		{
			"always true",
			func(a, b *Action) bool { return true },
			[][]*Action{
				{{Kind: Get}},
				{{Kind: Get}},
				{{Kind: Put}},
				{{Kind: Update}},
				{{Kind: Get}},
				{{Kind: Create}},
			},
		},
		{
			"Get vs. other",
			func(a, b *Action) bool { return (a.Kind == Get) != (b.Kind == Get) },
			[][]*Action{
				{{Kind: Get}, {Kind: Get}},
				{{Kind: Put}, {Kind: Update}},
				{{Kind: Get}},
				{{Kind: Create}},
			},
		},
	} {
		got := SplitActions(in, test.split)
		if diff := cmp.Diff(got, test.want, cmpopts.IgnoreFields(Action{}, "Doc")); diff != "" {
			t.Errorf("%s: %s", test.desc, diff)
		}
	}
}

func TestGroupActions(t *testing.T) {
	for _, test := range []struct {
		in   []*Action
		want [][]int // in the same order as the return args
	}{
		{
			in:   []*Action{{Kind: Get, Key: 1}},
			want: [][]int{nil, {0}, nil, nil},
		},
		{
			in: []*Action{
				{Kind: Get, Key: 1},
				{Kind: Get, Key: 3},
				{Kind: Put, Key: 1},
				{Kind: Replace, Key: 2},
				{Kind: Get, Key: 2},
			},
			want: [][]int{{0}, {1}, {2, 3}, {4}},
		},
		{
			in:   []*Action{{Kind: Create}, {Kind: Create}, {Kind: Create}},
			want: [][]int{nil, nil, {0, 1, 2}, nil},
		},
	} {
		got := make([][]*Action, 4)
		got[0], got[1], got[2], got[3] = GroupActions(test.in)
		want := make([][]*Action, 4)
		for i, s := range test.want {
			for _, x := range s {
				want[i] = append(want[i], test.in[x])
			}
		}
		diff := cmp.Diff(got, want,
			cmpopts.IgnoreUnexported(Document{}),
			cmpopts.SortSlices(func(a1, a2 *Action) bool {
				if a1.Kind != a2.Kind {
					return a1.Kind < a2.Kind
				}
				a1k, _ := a1.Key.(int)
				a2k, _ := a2.Key.(int)
				return a1k < a2k
			}))
		if diff != "" {
			t.Errorf("%v: %s", test.in, diff)
		}
	}
}

func (a *Action) String() string { // for TestGroupActions
	return fmt.Sprintf("<%s %v>", a.Kind, a.Key)
}

func TestAsFunc(t *testing.T) {
	x := 1
	as := AsFunc(x)

	var y int
	if !as(&y) || y != 1 {
		t.Errorf("*int: returned false or wrong value %d", y)
	}

	var z float64
	for _, arg := range []interface{}{nil, y, &z} {
		if as(arg) {
			t.Errorf("%#v: got true, want false", arg)
		}
	}
}

func TestGroupByFieldPath(t *testing.T) {
	for i, test := range []struct {
		in   []*Action
		want [][]int // indexes into test.in
	}{
		{
			in:   []*Action{{Index: 0}, {Index: 1}, {Index: 2}},
			want: [][]int{{0, 1, 2}},
		},
		{
			in:   []*Action{{Index: 0}, {Index: 1, FieldPaths: [][]string{{"a"}}}, {Index: 2}},
			want: [][]int{{0, 2}, {1}},
		},
		{
			in: []*Action{
				{Index: 0, FieldPaths: [][]string{{"a", "b"}}},
				{Index: 1, FieldPaths: [][]string{{"a"}}},
				{Index: 2},
				{Index: 3, FieldPaths: [][]string{{"a"}, {"b"}}},
			},
			want: [][]int{{0}, {1}, {2}, {3}},
		},
	} {
		got := GroupByFieldPath(test.in)
		want := make([][]*Action, len(test.want))
		for i, s := range test.want {
			want[i] = make([]*Action, len(s))
			for j, x := range s {
				want[i][j] = test.in[x]
			}
		}
		if diff := cmp.Diff(got, want, cmpopts.IgnoreUnexported(Document{})); diff != "" {
			t.Errorf("#%d: %s", i, diff)
		}
	}
}
