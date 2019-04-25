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
