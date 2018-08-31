// Copyright 2018 The Go Cloud Authors
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

package constantvar

import (
	"context"
	"reflect"
	"testing"
)

type sampleStruct struct {
	a string
	b int
}

// The normal conformance tests don't apply to constantvar since they can't
// be updated.
func TestVar(t *testing.T) {

	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		value interface{}
	}{
		{
			name:  "A string",
			value: "foo",
		},
		{
			name:  "Another string",
			value: "bar",
		},
		{
			name:  "A slice of bytes",
			value: []byte("hello world"),
		},
		{
			name:  "A struct",
			value: sampleStruct{a: "foo", b: 1},
		},
		{
			name:  "A struct pointer",
			value: &sampleStruct{a: "bar", b: 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := New(tc.value)
			snap, err := v.Watch(ctx)
			if err != nil {
				t.Fatal(err)
			}
			got := snap.Value
			if !reflect.DeepEqual(got, tc.value) {
				t.Errorf("got %v want %v", got, tc.value)
			}
		})
	}
}
