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

package dynamodocstore

import (
	"reflect"
	"testing"

	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/internal/docstore/driver"
)

func TestEncodeValue(t *testing.T) {
	av := func() *dyn.AttributeValue { return &dyn.AttributeValue{} }
	avn := func(s string) *dyn.AttributeValue { return av().SetN(s) }
	avl := func(avs ...*dyn.AttributeValue) *dyn.AttributeValue { return av().SetL(avs) }

	var seven int32 = 7
	var nullptr *int

	for _, test := range []struct {
		in   interface{}
		want *dyn.AttributeValue
	}{
		{nil, nullValue},
		{0, avn("0")},
		{uint64(999), avn("999")},
		{3.5, avn("3.5")},
		{"", nullValue},
		{"x", av().SetS("x")},
		{true, av().SetBOOL(true)},
		{nullptr, nullValue},
		{seven, avn("7")},
		{&seven, avn("7")},
		{3 + 4i, avl(avn("3"), avn("4"))},
		{[]int(nil), nullValue},
		{[]int{}, av().SetL([]*dyn.AttributeValue{})},
		{[]int{1, 2}, avl(avn("1"), avn("2"))},
		{[...]int{1, 2}, avl(avn("1"), avn("2"))},
		{[]interface{}{nil, false}, avl(nullValue, av().SetBOOL(false))},
		{map[string]int(nil), nullValue},
		{map[string]int{}, av().SetM(map[string]*dyn.AttributeValue{})},
		{
			map[string]int{"a": 1, "b": 2},
			av().SetM(map[string]*dyn.AttributeValue{
				"a": avn("1"),
				"b": avn("2"),
			}),
		},
	} {
		var e encoder
		if err := driver.Encode(reflect.ValueOf(test.in), &e); err != nil {
			t.Fatal(err)
		}
		got := e.av
		if !cmp.Equal(got, test.want, cmpopts.IgnoreUnexported(dyn.AttributeValue{})) {
			t.Errorf("%#v: got %#v, want %#v", test.in, got, test.want)
		}
	}
}
