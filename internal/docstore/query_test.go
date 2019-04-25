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

package docstore

import (
	"testing"

	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/docstore/driver"
)

func TestQueryValidFilter(t *testing.T) {
	for _, fp := range []string{"", ".a", "a..b", "b."} {
		q := Query{dq: &driver.Query{}}
		q.Where(fp, ">", 1)
		if got := gcerrors.Code(q.err); got != gcerrors.InvalidArgument {
			t.Errorf("fieldpath %q: got %s, want InvalidArgument", fp, got)
		}
	}
	q := Query{dq: &driver.Query{}}
	q.Where("a", "==", 1)
	if got := gcerrors.Code(q.err); got != gcerrors.InvalidArgument {
		t.Errorf("got %s, want InvalidArgument", got)
	}
	for _, v := range []interface{}{nil, 5 + 2i, []byte("x"), func() {}, []int{}, map[string]bool{}} {
		q := Query{dq: &driver.Query{}}
		q.Where("a", "=", v)
		if got := gcerrors.Code(q.err); got != gcerrors.InvalidArgument {
			t.Errorf("value %+v: got %s, want InvalidArgument", v, got)
		}
	}
}
