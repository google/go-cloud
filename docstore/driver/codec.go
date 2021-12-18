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

// TODO(jba): support struct tags.
// TODO(jba): for efficiency, enable encoding of only a subset of field paths.

package driver

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"

	"gocloud.dev/docstore/internal/fields"
	"gocloud.dev/internal/gcerr"
	"google.golang.org/protobuf/proto"
)

var (
	binaryMarshalerType   = reflect.TypeOf((*encoding.BinaryMarshaler)(nil)).Elem()
	binaryUnmarshalerType = reflect.TypeOf((*encoding.BinaryUnmarshaler)(nil)).Elem()
	textMarshalerType     = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshalerType   = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	protoMessageType      = reflect.TypeOf((*proto.Message)(nil)).Elem()
)

// An Encoder encodes Go values in some other form (e.g. JSON, protocol buffers).
// The encoding protocol is designed to avoid losing type information by passing
// values using interface{}. An Encoder is responsible for storing the value
// it is encoding.
//
// Because all drivers must support the same set of values, the encoding methods
// (with the exception of EncodeStruct) do not return errors. EncodeStruct is special
// because it is an escape hatch for arbitrary structs, not all of which may be
// encodable.
type Encoder interface {
	// These methods all encode and store a single Go value.
	EncodeNil()
	EncodeBool(bool)
	EncodeString(string)
	EncodeInt(int64)
	EncodeUint(uint64)
	EncodeFloat(float64)
	EncodeBytes([]byte)

	// EncodeList is called when a slice or array is encountered (except for a
	// []byte, which is handled by EncodeBytes). Its argument is the length of the
	// slice or array. The encoding algorithm will call the returned Encoder that
	// many times to encode the successive values of the list. After each such call,
	// ListIndex will be called with the index of the element just encoded.
	//
	// For example, []string{"a", "b"} will result in these calls:
	//     enc2 := enc.EncodeList(2)
	//     enc2.EncodeString("a")
	//     enc2.ListIndex(0)
	//     enc2.EncodeString("b")
	//     enc2.ListIndex(1)
	EncodeList(n int) Encoder
	ListIndex(i int)

	// EncodeMap is called when a map is encountered. Its argument is the number of
	// fields in the map. The encoding algorithm will call the returned Encoder that
	// many times to encode the successive values of the map. After each such call,
	// MapKey will be called with the key of the element just encoded.
	//
	// For example, map[string}int{"A": 1, "B": 2} will result in these calls:
	//     enc2 := enc.EncodeMap(2)
	//     enc2.EncodeInt(1)
	//     enc2.MapKey("A")
	//     enc2.EncodeInt(2)
	//     enc2.MapKey("B")
	//
	// EncodeMap is also called for structs. The map then consists of the exported
	// fields of the struct. For struct{A, B int}{1, 2}, if EncodeStruct returns
	// false, the same sequence of calls as above will occur.
	EncodeMap(n int) Encoder
	MapKey(string)

	// If the encoder wants to encode a value in a special way it should do so here
	// and return true along with any error from the encoding. Otherwise, it should
	// return false.
	EncodeSpecial(v reflect.Value) (bool, error)
}

// Encode encodes the value using the given Encoder. It traverses the value,
// iterating over arrays, slices, maps and the exported fields of structs. If it
// encounters a non-nil pointer, it encodes the value that it points to.
// Encode treats a few interfaces specially:
//
// If the value implements encoding.BinaryMarshaler, Encode invokes MarshalBinary
// on it and encodes the resulting byte slice.
//
// If the value implements encoding.TextMarshaler, Encode invokes MarshalText on it
// and encodes the resulting string.
//
// If the value implements proto.Message, Encode invokes proto.Marshal on it and encodes
// the resulting byte slice. Here proto is the package "google.golang.org/protobuf/proto".
//
// Not every map key type can be encoded. Only strings, integers (signed or
// unsigned), and types that implement encoding.TextMarshaler are permitted as map
// keys. These restrictions match exactly those of the encoding/json package.
func Encode(v reflect.Value, e Encoder) error {
	return wrap(encode(v, e), gcerr.InvalidArgument)
}

func encode(v reflect.Value, enc Encoder) error {
	if !v.IsValid() {
		enc.EncodeNil()
		return nil
	}
	done, err := enc.EncodeSpecial(v)
	if done {
		return err
	}
	if v.Type().Implements(binaryMarshalerType) {
		bytes, err := v.Interface().(encoding.BinaryMarshaler).MarshalBinary()
		if err != nil {
			return err
		}
		enc.EncodeBytes(bytes)
		return nil
	}
	if v.Type().Implements(protoMessageType) {
		if v.IsNil() {
			enc.EncodeNil()
		} else {
			bytes, err := proto.Marshal(v.Interface().(proto.Message))
			if err != nil {
				return err
			}
			enc.EncodeBytes(bytes)
		}
		return nil
	}
	if reflect.PtrTo(v.Type()).Implements(protoMessageType) {
		bytes, err := proto.Marshal(v.Addr().Interface().(proto.Message))
		if err != nil {
			return err
		}
		enc.EncodeBytes(bytes)
		return nil
	}
	if v.Type().Implements(textMarshalerType) {
		bytes, err := v.Interface().(encoding.TextMarshaler).MarshalText()
		if err != nil {
			return err
		}
		enc.EncodeString(string(bytes))
		return nil
	}
	switch v.Kind() {
	case reflect.Bool:
		enc.EncodeBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		enc.EncodeInt(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		enc.EncodeUint(v.Uint())
	case reflect.Float32, reflect.Float64:
		enc.EncodeFloat(v.Float())
	case reflect.String:
		enc.EncodeString(v.String())
	case reflect.Slice:
		if v.IsNil() {
			enc.EncodeNil()
			return nil
		}
		fallthrough
	case reflect.Array:
		return encodeList(v, enc)
	case reflect.Map:
		return encodeMap(v, enc)
	case reflect.Ptr:
		if v.IsNil() {
			enc.EncodeNil()
			return nil
		}
		return encode(v.Elem(), enc)
	case reflect.Interface:
		if v.IsNil() {
			enc.EncodeNil()
			return nil
		}
		return encode(v.Elem(), enc)

	case reflect.Struct:
		fields, err := fieldCache.Fields(v.Type())
		if err != nil {
			return err
		}
		return encodeStructWithFields(v, fields, enc)

	default:
		return gcerr.Newf(gcerr.InvalidArgument, nil, "cannot encode type %s", v.Type())
	}
	return nil
}

// Encode an array or non-nil slice.
func encodeList(v reflect.Value, enc Encoder) error {
	// Byte slices encode specially.
	if v.Type().Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
		enc.EncodeBytes(v.Bytes())
		return nil
	}
	n := v.Len()
	enc2 := enc.EncodeList(n)
	for i := 0; i < n; i++ {
		if err := encode(v.Index(i), enc2); err != nil {
			return err
		}
		enc2.ListIndex(i)
	}
	return nil
}

// Encode a map.
func encodeMap(v reflect.Value, enc Encoder) error {
	if v.IsNil() {
		enc.EncodeNil()
		return nil
	}
	keys := v.MapKeys()
	enc2 := enc.EncodeMap(len(keys))
	for _, k := range keys {
		sk, err := stringifyMapKey(k)
		if err != nil {
			return err
		}
		if err := encode(v.MapIndex(k), enc2); err != nil {
			return err
		}
		enc2.MapKey(sk)
	}
	return nil
}

// k is the key of a map. Encode it as a string.
// Only strings, integers (signed or unsigned), and types that implement
// encoding.TextMarshaler are supported.
func stringifyMapKey(k reflect.Value) (string, error) {
	// This is basically reflectWithString.resolve, from encoding/json/encode.go.
	if k.Kind() == reflect.String {
		return k.String(), nil
	}
	if tm, ok := k.Interface().(encoding.TextMarshaler); ok {
		b, err := tm.MarshalText()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	switch k.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	default:
		return "", gcerr.Newf(gcerr.InvalidArgument, nil, "cannot encode key %v of type %s", k, k.Type())
	}
}

func encodeStructWithFields(v reflect.Value, fields fields.List, e Encoder) error {
	e2 := e.EncodeMap(len(fields))
	for _, f := range fields {
		fv, ok := fieldByIndex(v, f.Index)
		if !ok {
			// if !ok, then f is a field in an embedded pointer to struct, and that embedded pointer
			// is nil in v. In other words, the field exists in the struct type, but not this particular
			// struct value. So we just ignore it.
			continue
		}
		if f.ParsedTag.(tagOptions).omitEmpty && IsEmptyValue(fv) {
			continue
		}
		if err := encode(fv, e2); err != nil {
			return err
		}
		e2.MapKey(f.Name)
	}
	return nil
}

// fieldByIndex retrieves the field of v at the given index if present.
// v must be a struct. index must refer to a valid field of v's type.
// The second return value is false if there is a nil embedded pointer
// along the path denoted by index.
//
// From encoding/json/encode.go.
func fieldByIndex(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

////////////////////////////////////////////////////////////////

// TODO(jba): consider a fast path: if we are decoding into a struct, assume the same struct
// was used to encode. Then we can build a map from field names to functions, where each
// function avoids all the tests of Decode and contains just the code for setting the field.

// TODO(jba): provide a way to override the check on missing fields.

// A Decoder decodes data that was produced by Encode back into Go values.
// Each Decoder instance is responsible for decoding one value.
type Decoder interface {
	// The AsXXX methods each report whether the value being decoded can be represented as
	// a particular Go type. If so, the method should return the value as that type, and true;
	// otherwise it should return the zero value and false.
	AsString() (string, bool)
	AsInt() (int64, bool)
	AsUint() (uint64, bool)
	AsFloat() (float64, bool)
	AsBytes() ([]byte, bool)
	AsBool() (bool, bool)
	AsNull() bool

	// ListLen should return the length of the value being decoded and true, if the
	// value can be decoded into a slice or array. Otherwise, ListLen should return
	// (0, false).
	ListLen() (int, bool)

	// If ListLen returned true, then DecodeList will be called. It should iterate
	// over the value being decoded in sequence from index 0, invoking the callback
	// for each element with the element's index and a Decoder for the element.
	// If the callback returns false, DecodeList should return immediately.
	DecodeList(func(int, Decoder) bool)

	// MapLen should return the number of fields of the value being decoded and true,
	// if the value can be decoded into a map or struct. Otherwise, it should return
	// (0, false).
	MapLen() (int, bool)

	// DecodeMap iterates over the fields of the value being decoded, invoke the
	// callback on each with field name, a Decoder for the field value, and a bool
	// to indicate whether or not to use exact match for the field names. It will
	// be called when MapLen returns true or decoding a struct. If the callback
	// returns false, DecodeMap should return immediately.
	DecodeMap(func(string, Decoder, bool) bool)

	// AsInterface should decode the value into the Go value that best represents it.
	AsInterface() (interface{}, error)

	// If the decoder wants to decode a value in a special way it should do so here
	// and return true, the decoded value, and any error from the decoding.
	// Otherwise, it should return false.
	AsSpecial(reflect.Value) (bool, interface{}, error)

	// String should return a human-readable representation of the Decoder, for error messages.
	String() string
}

// Decode decodes the value held in the Decoder d into v.
// Decode creates slices, maps and pointer elements as needed.
// It treats values that implement encoding.BinaryUnmarshaler,
// encoding.TextUnmarshaler and proto.Message specially; see Encode.
func Decode(v reflect.Value, d Decoder) error {
	return wrap(decode(v, d), gcerr.InvalidArgument)
}

func decode(v reflect.Value, d Decoder) error {
	if !v.CanSet() {
		return fmt.Errorf("while decoding: cannot set %+v", v)
	}
	// A Null value sets anything nullable to nil.
	// If the value isn't nullable, we keep going.
	// TODO(jba): should we treat decoding a null into a non-nullable as an error, or
	// ignore it like encoding/json does?
	if d.AsNull() {
		switch v.Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice:
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
	}

	if done, val, err := d.AsSpecial(v); done {
		if err != nil {
			return err
		}
		if reflect.TypeOf(val).AssignableTo(v.Type()) {
			v.Set(reflect.ValueOf(val))
			return nil
		}
		return decodingError(v, d)
	}

	// Handle implemented interfaces first.
	if reflect.PtrTo(v.Type()).Implements(binaryUnmarshalerType) {
		if b, ok := d.AsBytes(); ok {
			return v.Addr().Interface().(encoding.BinaryUnmarshaler).UnmarshalBinary(b)
		}
		return decodingError(v, d)
	}
	if reflect.PtrTo(v.Type()).Implements(protoMessageType) {
		if b, ok := d.AsBytes(); ok {
			return proto.Unmarshal(b, v.Addr().Interface().(proto.Message))
		}
		return decodingError(v, d)
	}
	if reflect.PtrTo(v.Type()).Implements(textUnmarshalerType) {
		if s, ok := d.AsString(); ok {
			return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
		return decodingError(v, d)
	}

	switch v.Kind() {
	case reflect.Bool:
		if b, ok := d.AsBool(); ok {
			v.SetBool(b)
			return nil
		}

	case reflect.String:
		if s, ok := d.AsString(); ok {
			v.SetString(s)
			return nil
		}

	case reflect.Float32, reflect.Float64:
		if f, ok := d.AsFloat(); ok {
			v.SetFloat(f)
			return nil
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, ok := d.AsInt()
		if !ok {
			// Accept a floating-point number with integral value.
			f, ok := d.AsFloat()
			if !ok {
				return decodingError(v, d)
			}
			i = int64(f)
			if float64(i) != f {
				return gcerr.Newf(gcerr.InvalidArgument, nil, "float %f does not fit into %s", f, v.Type())
			}
		}
		if v.OverflowInt(i) {
			return overflowError(i, v.Type())
		}
		v.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, ok := d.AsUint()
		if !ok {
			// Accept a floating-point number with integral value.
			f, ok := d.AsFloat()
			if !ok {
				return decodingError(v, d)
			}
			u = uint64(f)
			if float64(u) != f {
				return gcerr.Newf(gcerr.InvalidArgument, nil, "float %f does not fit into %s", f, v.Type())
			}
		}
		if v.OverflowUint(u) {
			return overflowError(u, v.Type())
		}
		v.SetUint(u)
		return nil

	case reflect.Slice, reflect.Array:
		return decodeList(v, d)

	case reflect.Map:
		return decodeMap(v, d)

	case reflect.Ptr:
		// If the pointer is nil, set it to a zero value.
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return decode(v.Elem(), d)

	case reflect.Struct:
		return decodeStruct(v, d)

	case reflect.Interface:
		if v.NumMethod() == 0 { // empty interface
			// If v holds a pointer, set the pointer.
			if !v.IsNil() && v.Elem().Kind() == reflect.Ptr {
				return decode(v.Elem(), d)
			}
			// Otherwise, create a fresh value.
			x, err := d.AsInterface()
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(x))
			return nil
		}
		// Any other kind of interface is an error???
	}

	return decodingError(v, d)
}

func decodeList(v reflect.Value, d Decoder) error {
	// If we're decoding into a byte slice or array, and the decoded value
	// supports that, then do the decoding.
	if v.Type().Elem().Kind() == reflect.Uint8 {
		if b, ok := d.AsBytes(); ok {
			if v.Kind() == reflect.Slice {
				v.SetBytes(b)
				return nil
			}
			// It's an array; copy the data in.
			err := prepareLength(v, len(b))
			if err != nil {
				return err
			}
			reflect.Copy(v, reflect.ValueOf(b))
			return nil
		}
		// Fall through to decode the []byte as an ordinary slice.
	}
	dlen, ok := d.ListLen()
	if !ok {
		return decodingError(v, d)
	}
	err := prepareLength(v, dlen)
	if err != nil {
		return err
	}
	d.DecodeList(func(i int, vd Decoder) bool {
		if err != nil || i >= dlen {
			return false
		}
		err = decode(v.Index(i), vd)
		return err == nil
	})
	return err
}

// v must be a slice or array. We want it to be of length wantLen. Prepare it as
// necessary (details described in the code below), and return its resulting length.
// If an array is too short, return an error. This behavior differs from
// encoding/json, which just populates a short array with whatever it can and drops
// the rest. That can lose data.
func prepareLength(v reflect.Value, wantLen int) error {
	vLen := v.Len()
	if v.Kind() == reflect.Slice {
		// Construct a slice of the right size, avoiding allocation if possible.
		switch {
		case vLen < wantLen: // v too short
			if v.Cap() >= wantLen { // extend its length if there's room
				v.SetLen(wantLen)
			} else { // else make a new one
				v.Set(reflect.MakeSlice(v.Type(), wantLen, wantLen))
			}
		case vLen > wantLen: // v too long; truncate it
			v.SetLen(wantLen)
		}
	} else { // array
		switch {
		case vLen < wantLen: // v too short
			return gcerr.Newf(gcerr.InvalidArgument, nil, "array length %d is too short for incoming list of length %d",
				vLen, wantLen)
		case vLen > wantLen: // v too long; set extra elements to zero
			z := reflect.Zero(v.Type().Elem())
			for i := wantLen; i < vLen; i++ {
				v.Index(i).Set(z)
			}
		}
	}
	return nil
}

// Since a map value is not settable via reflection, this function always creates a
// new element for each corresponding map key. Existing values of v are overwritten.
// This happens even if the map value is something like a pointer to a struct, where
// we could in theory populate the existing struct value instead of discarding it.
// This behavior matches encoding/json.
func decodeMap(v reflect.Value, d Decoder) error {
	mapLen, ok := d.MapLen()
	if !ok {
		return decodingError(v, d)
	}
	t := v.Type()
	if v.IsNil() {
		v.Set(reflect.MakeMapWithSize(t, mapLen))
	}
	et := t.Elem()
	var err error
	kt := v.Type().Key()
	d.DecodeMap(func(key string, vd Decoder, _ bool) bool {
		if err != nil {
			return false
		}
		el := reflect.New(et).Elem()
		err = decode(el, vd)
		if err != nil {
			return false
		}
		vk, e := unstringifyMapKey(key, kt)
		if e != nil {
			err = e
			return false
		}
		v.SetMapIndex(vk, el)
		return err == nil
	})
	return err
}

// Given a map key encoded as a string, and the type of the map key, convert the key
// into the type.
// For example, if we are decoding the key "3" for a map[int]interface{}, then key is "3"
// and keyType is reflect.Int.
func unstringifyMapKey(key string, keyType reflect.Type) (reflect.Value, error) {
	// This code is mostly from the middle of decodeState.object in encoding/json/decode.go.
	// Except for literalStore, which I don't understand.
	// TODO(jba): understand literalStore.
	switch {
	case keyType.Kind() == reflect.String:
		return reflect.ValueOf(key).Convert(keyType), nil
	case reflect.PtrTo(keyType).Implements(textUnmarshalerType):
		tu := reflect.New(keyType)
		if err := tu.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(key)); err != nil {
			return reflect.Value{}, err
		}
		return tu.Elem(), nil
	case keyType.Kind() == reflect.Interface && keyType.NumMethod() == 0:
		// TODO: remove this case? encoding/json doesn't support it.
		return reflect.ValueOf(key), nil
	default:
		switch keyType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			if reflect.Zero(keyType).OverflowInt(n) {
				return reflect.Value{}, overflowError(n, keyType)
			}
			return reflect.ValueOf(n).Convert(keyType), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			n, err := strconv.ParseUint(key, 10, 64)
			if err != nil {
				return reflect.Value{}, err
			}
			if reflect.Zero(keyType).OverflowUint(n) {
				return reflect.Value{}, overflowError(n, keyType)
			}
			return reflect.ValueOf(n).Convert(keyType), nil
		default:
			return reflect.Value{}, gcerr.Newf(gcerr.InvalidArgument, nil, "invalid key type %s", keyType)
		}
	}
}

func decodeStruct(v reflect.Value, d Decoder) error {
	fs, err := fieldCache.Fields(v.Type())
	if err != nil {
		return err
	}
	d.DecodeMap(func(key string, d2 Decoder, exactMatch bool) bool {
		if err != nil {
			return false
		}
		var f *fields.Field
		if exactMatch {
			f = fs.MatchExact(key)
		} else {
			f = fs.MatchFold(key)
		}
		if f == nil {
			err = gcerr.Newf(gcerr.InvalidArgument, nil, "no field matching %q in %s", key, v.Type())
			return false
		}
		fv, ok := fieldByIndexCreate(v, f.Index)
		if !ok {
			err = gcerr.Newf(gcerr.InvalidArgument, nil,
				"setting field %q in %s: cannot create embedded pointer field of unexported type",
				key, v.Type())
			return false
		}
		err = decode(fv, d2)
		return err == nil
	})
	return err
}

// fieldByIndexCreate retrieves the the field of v at the given index if present,
// creating embedded struct pointers where necessary.
// v must be a struct. index must refer to a valid field of v's type.
// The second return value is false If there is a nil embedded pointer of unexported
// type along the path denoted by index. (We cannot create such pointers.)
func fieldByIndexCreate(v reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				if !v.CanSet() {
					return reflect.Value{}, false
				}
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

func decodingError(v reflect.Value, d Decoder) error {
	return gcerr.New(gcerr.InvalidArgument, nil, 2, fmt.Sprintf("cannot set type %s to %v", v.Type(), d))
}

func overflowError(x interface{}, t reflect.Type) error {
	return gcerr.New(gcerr.InvalidArgument, nil, 2, fmt.Sprintf("value %v overflows type %s", x, t))
}

func wrap(err error, code gcerr.ErrorCode) error {
	if _, ok := err.(*gcerr.Error); !ok && err != nil {
		err = gcerr.New(code, err, 2, err.Error())
	}
	return err
}

var fieldCache = fields.NewCache(parseTag, nil, nil)

// IsEmptyValue returns whether or not v is a zero value of its type.
// Copied from encoding/json, go 1.12.
func IsEmptyValue(v reflect.Value) bool {
	switch k := v.Kind(); k {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

// Options for struct tags.
type tagOptions struct {
	omitEmpty bool // do not encode value if empty
}

// parseTag interprets docstore struct field tags.
func parseTag(t reflect.StructTag) (name string, keep bool, other interface{}, err error) {
	var opts []string
	name, keep, opts = fields.ParseStandardTag("docstore", t)
	tagOpts := tagOptions{}
	for _, opt := range opts {
		switch opt {
		case "omitempty":
			tagOpts.omitEmpty = true
		default:
			return "", false, nil, gcerr.Newf(gcerr.InvalidArgument, nil, "unknown tag option: %q", opt)
		}
	}
	return name, keep, tagOpts, nil
}
