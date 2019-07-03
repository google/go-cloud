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
	"gocloud.dev/gcerrors"
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

type badSpecial int

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
	Omit      int `docstore:"-"`
	OmitEmpty int `docstore:",omitempty"`
	Rename    int `docstore:"rename"`
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
		{[]byte{1, 2}, []byte{1, 2}},
		{[2]byte{3, 4}, []interface{}{uint64(3), uint64(4)}},
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
			map[uint]bool{18: true},
			map[string]interface{}{"18": true},
		},
		{
			map[te]bool{{'B'}: true},
			map[string]interface{}{"B": true},
		},
		{
			MyStruct{
				A:         1,
				B:         &tru,
				C:         []*te{{'T'}},
				D:         []time.Time{tm},
				T:         ts,
				Embed1:    Embed1{E1: "E1"},
				Embed2:    &Embed2{E2: "E2"},
				embed3:    embed3{E3: "E3"},
				embed4:    &embed4{E4: "E4"},
				Omit:      3,
				OmitEmpty: 4,
				Rename:    5,
			},
			map[string]interface{}{
				"A":         int64(1),
				"B":         true,
				"C":         []interface{}{"T"},
				"D":         []interface{}{tmb},
				"T":         tsb,
				"E1":        "E1",
				"E2":        "E2",
				"E3":        "E3",
				"E4":        "E4",
				"OmitEmpty": int64(4),
				"rename":    int64(5),
			},
		},
		{
			MyStruct{},
			map[string]interface{}{
				"A":      int64(0),
				"B":      nil,
				"C":      nil,
				"D":      nil,
				"T":      nil,
				"E1":     "",
				"E3":     "",
				"rename": int64(0),
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

type badBinaryMarshaler struct{}

func (badBinaryMarshaler) MarshalBinary() ([]byte, error) { return nil, errors.New("bad") }
func (*badBinaryMarshaler) UnmarshalBinary([]byte) error  { return errors.New("bad") }

type badTextMarshaler struct{}

func (badTextMarshaler) MarshalText() ([]byte, error) { return nil, errors.New("bad") }
func (*badTextMarshaler) UnmarshalText([]byte) error  { return errors.New("bad") }

func TestEncodeErrors(t *testing.T) {
	for _, test := range []struct {
		desc string
		val  interface{}
	}{
		{"MarshalBinary fails", badBinaryMarshaler{}},
		{"MarshalText fails", badTextMarshaler{}},
		{"bad type", make(chan int)},
		{"bad type in list", []interface{}{func() {}}},
		{"bad type in map", map[string]interface{}{"a": func() {}}},
		{"bad type in struct", &struct{ C chan int }{}},
		{"bad map key type", map[float32]int{1: 1}},
		{"MarshalText for map key fails", map[badTextMarshaler]int{{}: 1}},
	} {
		enc := &testEncoder{}
		if err := Encode(reflect.ValueOf(test.val), enc); err == nil {
			t.Errorf("%s: got nil, want error", test.desc)
		} else if c := gcerrors.Code(err); c != gcerrors.InvalidArgument {
			t.Errorf("%s: got code %s, want InvalidArgument", test.desc, c)
		}
	}
}

type testEncoder struct {
	val interface{}
}

func (e *testEncoder) EncodeNil()            { e.val = nil }
func (e *testEncoder) EncodeBool(x bool)     { e.val = x }
func (e *testEncoder) EncodeString(x string) { e.val = x }
func (e *testEncoder) EncodeInt(x int64)     { e.val = x }
func (e *testEncoder) EncodeUint(x uint64)   { e.val = x }
func (e *testEncoder) EncodeFloat(x float64) { e.val = x }
func (e *testEncoder) EncodeBytes(x []byte)  { e.val = x }

var (
	typeOfSpecial    = reflect.TypeOf(special(0))
	typeOfBadSpecial = reflect.TypeOf(badSpecial(0))
)

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
		in         interface{} // pointer that will be set
		val        interface{} // value to set it to
		want       interface{}
		exactMatch bool
	}{
		{new(interface{}), nil, nil, true},
		{new(int), int64(7), int(7), true},
		{new(uint8), uint64(250), uint8(250), true},
		{new(bool), true, true, true},
		{new(string), "x", "x", true},
		{new(float32), 4.25, float32(4.25), true},
		{new(*int), int64(2), &two, true},
		{new(*int), nil, (*int)(nil), true},
		{new([]byte), []byte("foo"), []byte("foo"), true},
		{new([]string), []interface{}{"a", "b"}, []string{"a", "b"}, true},
		{new([]**bool), []interface{}{true, false}, []**bool{&ptru, &pfa}, true},
		{&[1]int{1}, []interface{}{2}, [1]int{2}, true},
		{&[2]int{1, 2}, []interface{}{3}, [2]int{3, 0}, true}, // zero extra elements
		{&[]int{1, 2}, []interface{}{3}, []int{3}, true},      // truncate slice
		{
			// extend slice
			func() *[]int { s := make([]int, 1, 2); return &s }(),
			[]interface{}{5, 6},
			[]int{5, 6},
			true,
		},
		{
			new(map[string]string),
			map[string]interface{}{"a": "b"},
			map[string]string{"a": "b"},
			true,
		},
		{
			new(map[int]bool),
			map[string]interface{}{"17": true},
			map[int]bool{17: true},
			true,
		},
		{
			new(map[te]bool),
			map[string]interface{}{"B": true},
			map[te]bool{{'B'}: true},
			true,
		},
		{
			new(map[interface{}]bool),
			map[string]interface{}{"B": true},
			map[interface{}]bool{"B": true},
			true,
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
			true,
		},
		{new(special), 17, special(17), true},
		{new(myString), "x", myString("x"), true},
		{new([]myString), []interface{}{"x"}, []myString{"x"}, true},
		{new(time.Time), tmb, tm, true},
		{new(*time.Time), tmb, &tm, true},
		{new(*tspb.Timestamp), tsb, ts, true},
		{new([]time.Time), []interface{}{tmb}, []time.Time{tm}, true},
		{new([]*time.Time), []interface{}{tmb}, []*time.Time{&tm}, true},
		{
			new(map[myString]myString),
			map[string]interface{}{"a": "x"},
			map[myString]myString{"a": "x"},
			true,
		},
		{
			new(map[string]time.Time),
			map[string]interface{}{"t": tmb},
			map[string]time.Time{"t": tm},
			true,
		},
		{
			new(map[string]*time.Time),
			map[string]interface{}{"t": tmb},
			map[string]*time.Time{"t": &tm},
			true,
		},
		{new(te), "A", te{'A'}, true},
		{new(**te), "B", func() **te { x := &te{'B'}; return &x }(), true},

		{
			&MyStruct{embed4: &embed4{}},
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
			MyStruct{A: 1, B: &tru, C: []*te{{'T'}}, D: []time.Time{tm}, T: ts,
				Embed1: Embed1{E1: "E1"},
				Embed2: &Embed2{E2: "E2"},
				embed3: embed3{E3: "E3"},
				embed4: &embed4{E4: "E4"},
			},
			true,
		},
		{
			&MyStruct{embed4: &embed4{}},
			map[string]interface{}{
				"a":  int64(1),
				"b":  true,
				"c":  []interface{}{"T"},
				"d":  []interface{}{tmb},
				"t":  tsb,
				"e1": "E1",
				"e2": "E2",
				"e3": "E3",
				"e4": "E4",
			},
			MyStruct{A: 1, B: &tru, C: []*te{{'T'}}, D: []time.Time{tm}, T: ts,
				Embed1: Embed1{E1: "E1"},
				Embed2: &Embed2{E2: "E2"},
				embed3: embed3{E3: "E3"},
				embed4: &embed4{E4: "E4"},
			},
			false,
		},
	} {
		dec := &testDecoder{test.val, test.exactMatch}
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
			"bad type",
			new(int),
			"foo",
		},
		{
			"bad type in list",
			new([]int),
			[]interface{}{1, "foo"},
		},
		{
			"array too short",
			new([1]bool),
			[]interface{}{true, false},
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
		{
			"int overflow",
			new(int8),
			257,
		},
		{
			"uint overflow",
			new(uint8),
			uint(257),
		},
		{
			"non-integral float (int)",
			new(int),
			1.5,
		},
		{
			"non-integral float (uint)",
			new(uint),
			1.5,
		},
		{
			"bad special",
			new(badSpecial),
			badSpecial(0),
		},
		{
			"bad binary unmarshal",
			new(badBinaryMarshaler),
			[]byte{1},
		},
		{
			"binary unmarshal with non-byte slice",
			new(time.Time),
			1,
		},
		{
			"bad text unmarshal",
			new(badTextMarshaler),
			"foo",
		},
		{
			"text unmarshal with non-string",
			new(badTextMarshaler),
			1,
		},
		{
			"bad text unmarshal in map key",
			new(map[badTextMarshaler]int),
			map[string]interface{}{"a": 1},
		},
		{
			"bad int map key",
			new(map[int]int),
			map[string]interface{}{"a": 1},
		},
		{
			"overflow in int map key",
			new(map[int8]int),
			map[string]interface{}{"256": 1},
		},
		{
			"bad uint map key",
			new(map[uint]int),
			map[string]interface{}{"a": 1},
		},
		{
			"overflow in uint map key",
			new(map[uint8]int),
			map[string]interface{}{"256": 1},
		},
		{
			"case mismatch when decoding with exact match",
			&MyStruct{embed4: &embed4{}},
			map[string]interface{}{
				"a":  int64(1),
				"b":  true,
				"e1": "E1",
				"e2": "E2",
			},
		},
	} {
		dec := &testDecoder{test.val, true}
		err := Decode(reflect.ValueOf(test.in).Elem(), dec)
		if e, ok := err.(*gcerr.Error); !ok || err == nil || e.Code != gcerr.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument Error", test.desc, err)
		}
	}
}

func TestDecodeFail(t *testing.T) {
	// Verify that failure to decode a value results in an error.
	for _, in := range []interface{}{
		new(interface{}), new(bool), new(string), new(int), new(uint), new(float32),
		new([]byte), new([]int), new(map[string]interface{}),
	} {
		dec := &failDecoder{}
		err := Decode(reflect.ValueOf(in).Elem(), dec)
		if e, ok := err.(*gcerr.Error); !ok || err == nil || e.Code != gcerr.InvalidArgument {
			t.Errorf("%T: got %v, want InvalidArgument Error", in, err)
		}
	}
}

type testDecoder struct {
	val        interface{} // assume encoded by testEncoder.
	exactMatch bool
}

func (d testDecoder) String() string {
	return fmt.Sprintf("%+v of type %T", d.val, d.val)
}

func (d testDecoder) AsNull() bool {
	return d.val == nil
}

func (d testDecoder) AsBool() (bool, bool)     { x, ok := d.val.(bool); return x, ok }
func (d testDecoder) AsString() (string, bool) { x, ok := d.val.(string); return x, ok }

func (d testDecoder) AsInt() (int64, bool) {
	switch v := d.val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

func (d testDecoder) AsUint() (uint64, bool) {
	switch v := d.val.(type) {
	case uint64:
		return v, true
	case uint:
		return uint64(v), true
	default:
		return 0, false
	}
}

func (d testDecoder) AsFloat() (float64, bool) { x, ok := d.val.(float64); return x, ok }
func (d testDecoder) AsBytes() ([]byte, bool)  { x, ok := d.val.([]byte); return x, ok }

func (d testDecoder) ListLen() (int, bool) {
	l, ok := d.val.([]interface{})
	if !ok {
		return 0, false
	}
	return len(l), true
}

func (d testDecoder) DecodeList(f func(i int, vd Decoder) bool) {
	for i, v := range d.val.([]interface{}) {
		if !f(i, testDecoder{v, d.exactMatch}) {
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

func (d testDecoder) DecodeMap(f func(key string, vd Decoder, exactMatch bool) bool) {
	for k, v := range d.val.(map[string]interface{}) {
		if !f(k, testDecoder{v, d.exactMatch}, d.exactMatch) {
			break
		}
	}
}
func (d testDecoder) AsInterface() (interface{}, error) {
	return d.val, nil
}

func (d testDecoder) AsSpecial(v reflect.Value) (bool, interface{}, error) {
	switch v.Type() {
	case typeOfSpecial:
		return true, special(d.val.(int)), nil
	case typeOfBadSpecial:
		return true, nil, errors.New("bad special")
	default:
		return false, nil, nil
	}
}

// All of failDecoder's methods return failure.
type failDecoder struct{}

func (failDecoder) String() string                                       { return "failDecoder" }
func (failDecoder) AsNull() bool                                         { return false }
func (failDecoder) AsBool() (bool, bool)                                 { return false, false }
func (failDecoder) AsString() (string, bool)                             { return "", false }
func (failDecoder) AsInt() (int64, bool)                                 { return 0, false }
func (failDecoder) AsUint() (uint64, bool)                               { return 0, false }
func (failDecoder) AsFloat() (float64, bool)                             { return 0, false }
func (failDecoder) AsBytes() ([]byte, bool)                              { return nil, false }
func (failDecoder) ListLen() (int, bool)                                 { return 0, false }
func (failDecoder) DecodeList(func(i int, vd Decoder) bool)              { panic("impossible") }
func (failDecoder) MapLen() (int, bool)                                  { return 0, false }
func (failDecoder) DecodeMap(func(string, Decoder, bool) bool)           { panic("impossible") }
func (failDecoder) AsSpecial(v reflect.Value) (bool, interface{}, error) { return false, nil, nil }
func (failDecoder) AsInterface() (interface{}, error)                    { return nil, errors.New("fail") }
