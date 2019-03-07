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

package firedocstore

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore/driver"
)

// TODO(jba): Make this part of the conformance tests. (This differs from the current
// codec conformance test because it's a simple round trip using the docstore codec
// in both directions.)
func TestCodec(t *testing.T) {
	type S struct {
		N  *int
		I  int
		U  uint
		F  float64
		C  complex64
		St string
		B  bool
		By []byte
		L  []int
		M  map[string]bool
	}

	in := S{
		N:  nil,
		I:  1,
		U:  2,
		F:  2.5,
		C:  complex(9, 10),
		St: "foo",
		B:  true,
		L:  []int{3, 4, 5},
		M:  map[string]bool{"a": true, "b": false},
		By: []byte{6, 7, 8},
	}
	ev, err := encodeValue(in)
	if err != nil {
		t.Fatal(err)
	}
	var got S
	if err := driver.Decode(reflect.ValueOf(&got), decoder{ev}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got, in); diff != "" {
		t.Error(diff)
	}
}
