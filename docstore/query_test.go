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
	"strings"
	"testing"

	"gocloud.dev/docstore/driver"
	"gocloud.dev/gcerrors"
)

func TestQueryValidFilter(t *testing.T) {
	for _, fp := range []FieldPath{"", ".a", "a..b", "b."} {
		q := Query{dq: &driver.Query{}}
		q.Where(fp, ">", 1)
		if got := gcerrors.Code(q.err); got != gcerrors.InvalidArgument {
			t.Errorf("fieldpath %q: got %s, want InvalidArgument", fp, got)
		}
	}
	for _, op := range []string{"==", "!="} {
		q := Query{dq: &driver.Query{}}
		q.Where("a", op, 1)
		if got := gcerrors.Code(q.err); got != gcerrors.InvalidArgument {
			t.Errorf("op %s: got %s, want InvalidArgument", op, got)
		}
	}
	for _, v := range []interface{}{nil, 5 + 2i, []byte("x"), func() {}, []int{}, map[string]bool{}} {
		q := Query{dq: &driver.Query{}}
		q.Where("a", "=", v)
		if got := gcerrors.Code(q.err); got != gcerrors.InvalidArgument {
			t.Errorf("value %+v: got %s, want InvalidArgument", v, got)
		}
	}
}

func TestInvalidQuery(t *testing.T) {
	ctx := context.Background()
	// We detect that these queries are invalid before they reach the driver.
	c := &Collection{}

	for _, test := range []struct {
		desc         string
		appliesToGet bool
		q            *Query
		contains     string // error text must contain this string
	}{
		{"negative Limit", true, c.Query().Limit(-1), "limit"},
		{"zero Limit", true, c.Query().Limit(0), "limit"},
		{"two Limits", true, c.Query().Limit(1).Limit(2), "limit"},
		{"empty OrderBy field", true, c.Query().OrderBy("", Ascending), "empty field"},
		{"bad OrderBy direction", true, c.Query().OrderBy("x", "y"), "direction"},
		{"two OrderBys", true, c.Query().OrderBy("x", Ascending).OrderBy("y", Descending), "orderby"},
		{"OrderBy not in Where", true, c.Query().OrderBy("x", Ascending).Where("y", ">", 1), "orderby"},
		{"any Limit", false, c.Query().Limit(1), "limit"},
		{"any OrderBy", false, c.Query().OrderBy("x", Descending), "orderby"},
	} {
		check := func(err error) {
			if gcerrors.Code(err) != gcerrors.InvalidArgument {
				t.Errorf("%s: got %v, want InvalidArgument", test.desc, err)
				return
			}
			if !strings.Contains(strings.ToLower(err.Error()), test.contains) {
				t.Errorf("%s: got %q, wanted it to contain %q", test.desc, err.Error(), test.contains)
			}
		}
		if test.appliesToGet {
			check(test.q.Get(ctx).Next(ctx, nil))
		}
	}
}
