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
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"gocloud.dev/internal/gcerr"
)

type myString string

type te struct{ X byte }

func (e te) MarshalText() ([]byte, error) {
	return []byte{e.X}, nil
}

func (e *te) UnmarshalText(b []byte) error {
	if len(b) != 1 {
		return errors.New("te.UnmarshalText: need exactly 1 byte")
	}
	e.X = b[0]
	return nil
}

type special int

func (special) MarshalBinary() ([]byte, error) { panic("should never be called") }
func (*special) UnmarshalBinary([]byte) error  { panic("should never be called") }

type Embed1 struct {
	E1 string
}
type Embed2 struct {
	E2 string
}

type embed3 struct {
	E3 string
}

type embed4 struct {
	E4 string
}

type MyStruct struct {
	A int
	B *bool
	C []*te
	D []time.Time
	T *tspb.Timestamp
	Embed1
	*Embed2
	embed3
	*embed4
}

func TestEncode(t *testing.T) {
	var seven int32 = 7
	var nullptr *int
	tm := time.Now()
	tmb, err := tm.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	tru := true
	ts := &tspb.Timestamp{Seconds: 25, Nanos: 300}
	tsb, err := proto.Marshal(ts)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		in, want interface{}
	}{
		{nil, nil},
		{0, int64(0)},
		{uint64(999), uint64(999)},
		{float32(3.5), float64(3.5)},
		{"", ""},
		{"x", "x"},
		{true, true},
		{nullptr, nil},
		{seven, int64(seven)},
		{&seven, int64(seven)},
		{3 + 4i, 3 + 4i},
		{[]int(nil), nil},
		{[]int{}, []interface{}{}},
		{[]int{1, 2}, []interface{}{int64(1), int64(2)}},
		{
			[][]string{{"a", "b"}, {"c", "d"}},
			[]interface{}{
				[]interface{}{"a", "b"},
				[]interface{}{"c", "d"},
			},
		},
		{[...]int{1, 2}, []interface{}{int64(1), int64(2)}},
		{[]interface{}{nil, false}, []interface{}{nil, false}},
		{map[string]int(nil), nil},
		{map[string]int{}, map[string]interface{}{}},
		{
			map[string]int{"a": 1, "b": 2},
			map[string]interface{}{"a": int64(1), "b": int64(2)},
		},
		{tm, tmb},
		{ts, tsb},
		{te{'A'}, "A"},
		{special(17), 17},
		{myString("x"), "x"},
		{[]myString{"x"}, []interface{}{"x"}},
		{map[myString]myString{"a": "x"}, map[string]interface{}{"a": "x"}},
		{
			map[int]bool{17: true},
			map[string]interface{}{"17": true},
		},
		{
			map[te]bool{{'B'}: true},
			map[string]interface{}{"B": true},
		},
		{
			MyStruct{
				A:      1,
				B:      &tru,
				C:      []*te{{'T'}},
				D:      []time.Time{tm},
				T:      ts,
				Embed1: Embed1{E1: "E1"},
				Embed2: &Embed2{E2: "E2"},
				embed3: embed3{E3: "E3"},
				embed4: &embed4{E4: "E4"},
			},
			map[string]interface{}{
				"A":  int64(1),
				"B":  true,
				"C":  []interface{}{"T"},
				"D":  []interface{}{tmb},
				"T":  tsb,
				"E1": "E1",
				"E2": "E2",
				"E3": "E3",
				"E4": "E4",
			},
		},
		{
			MyStruct{},
			map[string]interface{}{
				"A":  int64(0),
				"B":  nil,
				"C":  nil,
				"D":  nil,
				"T":  nil,
				"E1": "",
				"E3": "",
			},
		},
	} {
		enc := &testEncoder{}
		if err := Encode(reflect.ValueOf(test.in), enc); err != nil {
			t.Fatal(err)
		}
		got := enc.val
		if diff := cmp.Diff(got, test.want); diff != "" {
			t.Errorf("%#v (got=-, want=+):\n%s", test.in, diff)
		}
	}
}

type testEncoder struct {
	val interface{}
}

func (e *testEncoder) EncodeNil()                 { e.val = nil }
func (e *testEncoder) EncodeBool(x bool)          { e.val = x }
func (e *testEncoder) EncodeString(x string)      { e.val = x }
func (e *testEncoder) EncodeInt(x int64)          { e.val = x }
func (e *testEncoder) EncodeUint(x uint64)        { e.val = x }
func (e *testEncoder) EncodeFloat(x float64)      { e.val = x }
func (e *testEncoder) EncodeComplex(x complex128) { e.val = x }
func (e *testEncoder) EncodeBytes(x []byte)       { e.val = x }

var typeOfSpecial = reflect.TypeOf(special(0))

func (e *testEncoder) EncodeSpecial(v reflect.Value) (bool, error) {
	// special would normally encode as a []byte, because it implements BinaryMarshaler.
	// Encode it as an int instead.
	if v.Type() == typeOfSpecial {
		e.val = int(v.Interface().(special))
		return true, nil
	}
	return false, nil
}

func (e *testEncoder) ListIndex(int) { panic("impossible") }
func (e *testEncoder) MapKey(string) { panic("impossible") }

func (e *testEncoder) EncodeList(n int) Encoder {
	s := make([]interface{}, n)
	e.val = s
	return &listEncoder{s: s}
}

func (e *testEncoder) EncodeMap(n int) Encoder {
	m := make(map[string]interface{}, n)
	e.val = m
	return &mapEncoder{m: m}
}

type listEncoder struct {
	s []interface{}
	testEncoder
}

func (e *listEncoder) ListIndex(i int) { e.s[i] = e.val }

type mapEncoder struct {
	m map[string]interface{}
	testEncoder
}

func (e *mapEncoder) MapKey(k string) { e.m[k] = e.val }

func TestDecode(t *testing.T) {
	two := 2
	tru := true
	fa := false
	ptru := &tru
	pfa := &fa
	tm := time.Now()
	tmb, err := tm.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	ts := &tspb.Timestamp{Seconds: 25, Nanos: 300}
	tsb, err := proto.Marshal(ts)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		in   interface{} // pointer that will be set
		val  interface{} // value to set it to
		want interface{}
	}{
		{new(interface{}), nil, nil},
		{new(int), int64(7), int(7)},
		{new(uint8), uint64(250), uint8(250)},
		{new(bool), true, true},
		{new(string), "x", "x"},
		{new(float32), 4.25, float32(4.25)},
		{new(complex64), 3 + 5i, complex64(3 + 5i)},
		{new(*int), int64(2), &two},
		{new(*int), nil, (*int)(nil)},
		{new([]byte), []byte("foo"), []byte("foo")},
		{new([]string), []interface{}{"a", "b"}, []string{"a", "b"}},
		{new([]**bool), []interface{}{true, false}, []**bool{&ptru, &pfa}},
		{new(special), 17, special(17)},
		{
			new(map[string]string),
			map[string]interface{}{"a": "b"},
			map[string]string{"a": "b"},
		},
		{
			new(map[int]bool),
			map[string]interface{}{"17": true},
			map[int]bool{17: true},
		},
		{
			new(map[te]bool),
			map[string]interface{}{"B": true},
			map[te]bool{{'B'}: true},
		},
		{
			new(map[interface{}]bool),
			map[string]interface{}{"B": true},
			map[interface{}]bool{"B": true},
		},
		{
			new(map[string][]bool),
			map[string]interface{}{
				"a": []interface{}{true, false},
				"b": []interface{}{false, true},
			},
			map[string][]bool{
				"a": {true, false},
				"b": {false, true},
			},
		},
		{new(myString), "x", myString("x")},
		{new([]myString), []interface{}{"x"}, []myString{"x"}},
		{new(time.Time), tmb, tm},
		{new(*time.Time), tmb, &tm},
		{new(*tspb.Timestamp), tsb, ts},
		{new([]time.Time), []interface{}{tmb}, []time.Time{tm}},
		{new([]*time.Time), []interface{}{tmb}, []*time.Time{&tm}},
		{
			new(map[myString]myString),
			map[string]interface{}{"a": "x"},
			map[myString]myString{"a": "x"},
		},
		{
			new(map[string]time.Time),
			map[string]interface{}{"t": tmb},
			map[string]time.Time{"t": tm},
		},
		{
			new(map[string]*time.Time),
			map[string]interface{}{"t": tmb},
			map[string]*time.Time{"t": &tm},
		},
		{new(te), "A", te{'A'}},
		{new(**te), "B", func() **te { x := &te{'B'}; return &x }()},

		{
			&MyStruct{embed4: &embed4{}},
			map[string]interface{}{
				"a":  int64(1),
				"b":  true,
				"C":  []interface{}{"T"},
				"D":  []interface{}{tmb},
				"T":  tsb,
				"E1": "E1",
				"E2": "E2",
				"E3": "E3",
				"E4": "E4",
			},
			MyStruct{A: 1, B: &tru, C: []*te{{'T'}}, D: []time.Time{tm}, T: ts,
				Embed1: Embed1{E1: "E1"},
				Embed2: &Embed2{E2: "E2"},
				embed3: embed3{E3: "E3"},
				embed4: &embed4{E4: "E4"},
			},
		},
	} {
		dec := &testDecoder{test.val}
		if err := Decode(reflect.ValueOf(test.in).Elem(), dec); err != nil {
			t.Fatalf("%T: %v", test.in, err)
		}
		got := reflect.ValueOf(test.in).Elem().Interface()
		diff := cmp.Diff(got, test.want, cmp.Comparer(proto.Equal), cmp.AllowUnexported(MyStruct{}))
		if diff != "" {
			t.Errorf("%T (got=-, want=+): %s", test.in, diff)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	for _, test := range []struct {
		desc    string
		in, val interface{}
	}{
		{
			"array too short",
			new([1]bool),
			[]bool{true, false},
		},
		{
			"bad map key type",
			new(map[float64]interface{}),
			map[string]interface{}{"a": 1},
		},
		{
			"unknown struct field",
			new(MyStruct),
			map[string]interface{}{"bad": 1},
		},
		{
			"nil embedded, unexported pointer to struct",
			new(MyStruct),
			map[string]interface{}{"E4": "E4"},
		},
	} {
		dec := &testDecoder{test.val}
		err := Decode(reflect.ValueOf(test.in).Elem(), dec)
		if e, ok := err.(*gcerr.Error); !ok || err == nil || e.Code != gcerr.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument Error", test.desc, err)
			fmt.Printf("%+v\n", test.in)
		}
	}
}

type testDecoder struct {
	val interface{} // assume encoded by testEncoder.
}

func (d testDecoder) String() string {
	return fmt.Sprint(d.val)
}

func (d testDecoder) AsNull() bool {
	return d.val == nil
}

func (d testDecoder) AsBool() (bool, bool)          { x, ok := d.val.(bool); return x, ok }
func (d testDecoder) AsString() (string, bool)      { x, ok := d.val.(string); return x, ok }
func (d testDecoder) AsInt() (int64, bool)          { x, ok := d.val.(int64); return x, ok }
func (d testDecoder) AsUint() (uint64, bool)        { x, ok := d.val.(uint64); return x, ok }
func (d testDecoder) AsFloat() (float64, bool)      { x, ok := d.val.(float64); return x, ok }
func (d testDecoder) AsComplex() (complex128, bool) { x, ok := d.val.(complex128); return x, ok }
func (d testDecoder) AsBytes() ([]byte, bool)       { x, ok := d.val.([]byte); return x, ok }

func (d testDecoder) ListLen() (int, bool) {
	l, ok := d.val.([]interface{})
	if !ok {
		return 0, false
	}
	return len(l), true
}

func (d testDecoder) DecodeList(f func(i int, vd Decoder) bool) {
	for i, v := range d.val.([]interface{}) {
		if !f(i, testDecoder{v}) {
			break
		}
	}
}
func (d testDecoder) MapLen() (int, bool) {
	if m, ok := d.val.(map[string]interface{}); ok {
		return len(m), true
	}
	return 0, false
}

func (d testDecoder) DecodeMap(f func(key string, vd Decoder) bool) {
	for k, v := range d.val.(map[string]interface{}) {
		if !f(k, testDecoder{v}) {
			break
		}
	}
}
func (d testDecoder) AsInterface() (interface{}, error) {
	return d.val, nil
}

func (d testDecoder) AsSpecial(v reflect.Value) (bool, interface{}, error) {
	if v.Type() == typeOfSpecial {
		return true, special(d.val.(int)), nil
	}
	return false, nil, nil
}
