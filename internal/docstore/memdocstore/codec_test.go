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

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/docstore/driver"
)

type aStruct struct {
	X int
	embed
	Z *bool
	W uint
}

type embed struct {
	Y string
}

func TestEncodeDoc(t *testing.T) {
	var b bool = true
	for _, test := range []struct {
		in   interface{}
		want map[string]interface{}
	}{
		{
			map[string]interface{}{
				"x": map[int]interface{}{
					1: "a",
					2: 17,
					3: []float32{1.0, 2.5},
					4: map[string]bool{"false": false, "true": true},
				},
			},
			map[string]interface{}{
				"x": map[string]interface{}{
					"1": "a",
					"2": int64(17),
					"3": []interface{}{float64(1.0), float64(2.5)},
					"4": map[string]interface{}{"false": false, "true": true},
				},
			},
		},
		{
			&aStruct{
				X:     3,
				embed: embed{Y: "y"},
				Z:     &b,
				W:     33,
			},
			map[string]interface{}{
				"X": int64(3),
				"Y": "y",
				"Z": true,
				"W": int64(33),
			},
		},
	} {
		doc, err := driver.NewDocument(test.in)
		if err != nil {
			t.Fatal(err)
		}
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
	for _, test := range []struct {
		in   map[string]interface{}
		val  interface{}
		want interface{}
	}{
		{
			map[string]interface{}{
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
			map[string]interface{}{
				"X": int64(3),
				"Y": "y",
				"Z": true,
				"W": int64(33),
			},
			&aStruct{},
			&aStruct{
				X:     3,
				embed: embed{Y: "y"},
				Z:     &b,
				W:     33,
			},
		},
	} {
		got := test.val
		doc, err := driver.NewDocument(test.val)
		if err != nil {
			t.Fatal(err)
		}
		if err := decodeDoc(doc, test.in); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got, test.want, cmp.AllowUnexported(aStruct{})); diff != "" {
			t.Errorf("%+v: %s", test.in, diff)
		}
	}
}
