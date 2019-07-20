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

package memdocstore

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/docstore/drivertest"
)

type aStruct struct {
	X int
	embed
	Z *bool
	W uint
	T time.Time
	L []int
	F float32
	B []byte
}

type embed struct {
	Y string
}

func TestEncodeDoc(t *testing.T) {
	var b bool = true
	tm := time.Now()
	for _, test := range []struct {
		in   interface{}
		want storedDoc
	}{
		{
			in: map[string]interface{}{
				"x": map[int]interface{}{
					1: "a",
					2: 17,
					3: []float32{1.0, 2.5},
					4: map[string]bool{"false": false, "true": true},
				},
			},
			want: storedDoc{
				"x": map[string]interface{}{
					"1": "a",
					"2": int64(17),
					"3": []interface{}{float64(1.0), float64(2.5)},
					"4": map[string]interface{}{"false": false, "true": true},
				},
			},
		},
		{
			in: &aStruct{
				X:     3,
				embed: embed{Y: "y"},
				Z:     &b,
				W:     33,
				T:     tm,
				L:     []int{4, 5},
				F:     2.5,
				B:     []byte("abc"),
			},
			want: storedDoc{
				"X": int64(3),
				"Y": "y",
				"Z": true,
				"W": int64(33),
				"T": tm,
				"L": []interface{}{int64(4), int64(5)},
				"F": 2.5,
				"B": []byte("abc"),
			},
		},
	} {
		doc := drivertest.MustDocument(test.in)
		got, err := encodeDoc(doc)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got, test.want); diff != "" {
			t.Errorf("%+v: %s", test.in, diff)
		}
	}
}

func TestDecodeDoc(t *testing.T) {
	var b bool = true
	tm := time.Now()
	for _, test := range []struct {
		in   storedDoc
		val  interface{}
		want interface{}
	}{
		{
			storedDoc{
				"x": map[string]interface{}{
					"1": "a",
					"2": int64(17),
					"3": []interface{}{float64(1.0), float64(2.5)},
					"4": map[string]interface{}{"false": false, "true": true},
				},
			},
			map[string]interface{}{},
			map[string]interface{}{
				"x": map[string]interface{}{
					"1": "a",
					"2": int64(17),
					"3": []interface{}{1.0, 2.5},
					"4": map[string]interface{}{"false": false, "true": true},
				},
			},
		},
		{
			storedDoc{
				"X": int64(3),
				"Y": "y",
				"Z": true,
				"W": int64(33),
				"T": tm,
				"L": []interface{}{int64(4), int64(5)},
				"F": 2.5,
				"B": []byte("abc"),
			},
			&aStruct{},
			&aStruct{
				X:     3,
				embed: embed{Y: "y"},
				Z:     &b,
				W:     33,
				T:     tm,
				L:     []int{4, 5},
				F:     2.5,
				B:     []byte("abc"),
			},
		},
	} {
		got := test.val
		doc := drivertest.MustDocument(test.val)
		if err := decodeDoc(test.in, doc, nil); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got, test.want, cmp.AllowUnexported(aStruct{})); diff != "" {
			t.Errorf("%+v: %s", test.in, diff)
		}
	}
}
