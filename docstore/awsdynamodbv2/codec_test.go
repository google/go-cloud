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

package awsdynamodb

import (
	"reflect"
	"testing"

	dyn2Types "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	// dynattr "github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gocloud.dev/docstore/driver"
	// "gocloud.dev/docstore/drivertest"
)

func TestEncodeValue(t *testing.T) {
	avn := func(s string) dyn2Types.AttributeValue { return &dyn2Types.AttributeValueMemberN{Value: s} }
	avl := func(avs ...dyn2Types.AttributeValue) dyn2Types.AttributeValue { return &dyn2Types.AttributeValueMemberL{Value: avs} }

	var seven int32 = 7
	var nullptr *int

	for _, test := range []struct {
		in   interface{}
		want dyn2Types.AttributeValue
	}{
		{nil, nullValue},
		{0, avn("0")},
		{uint64(999), avn("999")},
		{3.5, avn("3.5")},
		{"", nullValue},
		{"x", &dyn2Types.AttributeValueMemberS{Value: "x"}},
		{true, &dyn2Types.AttributeValueMemberBOOL{Value: true}},
		{nullptr, nullValue},
		{seven, avn("7")},
		{&seven, avn("7")},
		{[]int(nil), nullValue},
		{[]int{}, &dyn2Types.AttributeValueMemberL{Value: []dyn2Types.AttributeValue{}}},
		{[]int{1, 2}, avl(avn("1"), avn("2"))},
		{[...]int{1, 2}, avl(avn("1"), avn("2"))},
		{[]interface{}{nil, false}, avl(nullValue, &dyn2Types.AttributeValueMemberBOOL{Value: false})},
		{map[string]int(nil), nullValue},
		{map[string]int{}, &dyn2Types.AttributeValueMemberM{Value: map[string]dyn2Types.AttributeValue{}}},
		{
			map[string]int{"a": 1, "b": 2},
			&dyn2Types.AttributeValueMemberM{Value: map[string]dyn2Types.AttributeValue{
				"a": avn("1"),
				"b": avn("2"),
			}},
		},
	} {
		var e encoder
		if err := driver.Encode(reflect.ValueOf(test.in), &e); err != nil {
			t.Fatal(err)
		}
		got := e.av
		if !cmp.Equal(got, test.want, cmpopts.IgnoreUnexported(
			dyn2Types.AttributeValueMemberB{},
			dyn2Types.AttributeValueMemberBOOL{},
			dyn2Types.AttributeValueMemberBS{},
			dyn2Types.AttributeValueMemberL{},
			dyn2Types.AttributeValueMemberM{},
			dyn2Types.AttributeValueMemberN{},
			dyn2Types.AttributeValueMemberNS{},
			dyn2Types.AttributeValueMemberNULL{},
			dyn2Types.AttributeValueMemberS{},
			dyn2Types.AttributeValueMemberSS{},
		)) {
			t.Errorf("%#v: got %#v, want %#v", test.in, got, test.want)
		}
	}
}

func TestDecodeErrorOnUnsupported(t *testing.T) {
	for _, tc := range []struct {
		in  dyn2Types.AttributeValue
		out interface{}
	}{
		{&dyn2Types.AttributeValueMemberSS{Value: []string{"foo", "bar"}}, []string{}},
		{&dyn2Types.AttributeValueMemberNS{Value: []string{"1.1", "-2.2", "3.3"}}, []float64{}},
		{&dyn2Types.AttributeValueMemberBS{Value: [][]byte{{4}, {5}, {6}}}, [][]byte{}},
	} {
		d := decoder{av: tc.in}
		if err := driver.Decode(reflect.ValueOf(tc.out), &d); err == nil {
			t.Error("got nil error, want unsupported error")
		}
	}
}

// type codecTester struct{}

// func (ct *codecTester) UnsupportedTypes() []drivertest.UnsupportedType {
// 	return []drivertest.UnsupportedType{drivertest.BinarySet}
// }

// func (ct *codecTester) NativeEncode(obj interface{}) (interface{}, error) {
// 	return dynattr.Marshal(obj)
// }

// func (ct *codecTester) NativeDecode(value, dest interface{}) error {
// 	return dynattr.Unmarshal(value.(*dyn.AttributeValue), dest)
// }

// func (ct *codecTester) DocstoreEncode(obj interface{}) (interface{}, error) {
// 	return encodeDoc(drivertest.MustDocument(obj))
// }

// func (ct *codecTester) DocstoreDecode(value, dest interface{}) error {
// 	return decodeDoc(value.(*dyn.AttributeValue), drivertest.MustDocument(dest))
// }
