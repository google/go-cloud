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
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	dyn2Types "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gocloud.dev/docstore/driver"
)

var nullValue = &dyn2Types.AttributeValueMemberNULL{Value: true}

type encoder struct {
	av dyn2Types.AttributeValue
}

func (e *encoder) EncodeNil()        { e.av = nullValue }
func (e *encoder) EncodeBool(x bool) { e.av = &dyn2Types.AttributeValueMemberBOOL{Value: x} }
func (e *encoder) EncodeInt(x int64) {
	e.av = &dyn2Types.AttributeValueMemberN{Value: strconv.FormatInt(x, 10)}
}
func (e *encoder) EncodeUint(x uint64) {
	e.av = &dyn2Types.AttributeValueMemberN{Value: strconv.FormatUint(x, 10)}
}
func (e *encoder) EncodeBytes(x []byte)  { e.av = &dyn2Types.AttributeValueMemberB{Value: x} }
func (e *encoder) EncodeFloat(x float64) { e.av = encodeFloat(x) }

func (e *encoder) ListIndex(int) { panic("impossible") }
func (e *encoder) MapKey(string) { panic("impossible") }

func (e *encoder) EncodeString(x string) {
	if len(x) == 0 {
		e.av = nullValue
	} else {
		e.av = &dyn2Types.AttributeValueMemberS{Value: x}
	}
}

func (e *encoder) EncodeComplex(x complex128) {
	e.av = &dyn2Types.AttributeValueMemberL{Value: []dyn2Types.AttributeValue{encodeFloat(real(x)), encodeFloat(imag(x))}}
}

func (e *encoder) EncodeList(n int) driver.Encoder {
	s := make([]dyn2Types.AttributeValue, n)
	e.av = &dyn2Types.AttributeValueMemberL{Value: s}
	return &listEncoder{s: s}
}

func (e *encoder) EncodeMap(n int) driver.Encoder {
	m := make(map[string]dyn2Types.AttributeValue, n)
	e.av = &dyn2Types.AttributeValueMemberM{Value: m}
	return &mapEncoder{m: m}
}

var typeOfGoTime = reflect.TypeOf(time.Time{})

// EncodeSpecial encodes time.Time specially.
func (e *encoder) EncodeSpecial(v reflect.Value) (bool, error) {
	switch v.Type() {
	case typeOfGoTime:
		ts := v.Interface().(time.Time).Format(time.RFC3339Nano)
		e.EncodeString(ts)
	default:
		return false, nil
	}
	return true, nil
}

type listEncoder struct {
	s []dyn2Types.AttributeValue
	encoder
}

func (e *listEncoder) ListIndex(i int) { e.s[i] = e.av }

type mapEncoder struct {
	m map[string]dyn2Types.AttributeValue
	encoder
}

func (e *mapEncoder) MapKey(k string) { e.m[k] = e.av }

func encodeDoc(doc driver.Document) (dyn2Types.AttributeValue, error) {
	var e encoder
	if err := doc.Encode(&e); err != nil {
		return nil, err
	}
	return e.av, nil
}

// Encode the key fields of the given document into a map AttributeValue.
// pkey and skey are the names of the partition key field and the sort key field.
// pkey must always be non-empty, but skey may be empty if the collection has no sort key.
func encodeDocKeyFields(doc driver.Document, pkey, skey string) (*dyn2Types.AttributeValueMemberM, error) {
	m := map[string]dyn2Types.AttributeValue{}

	set := func(fieldName string) error {
		fieldVal, err := doc.GetField(fieldName)
		if err != nil {
			return err
		}
		attrVal, err := encodeValue(fieldVal)
		if err != nil {
			return err
		}
		m[fieldName] = attrVal
		return nil
	}

	if err := set(pkey); err != nil {
		return nil, err
	}
	if skey != "" {
		if err := set(skey); err != nil {
			return nil, err
		}
	}
	return &dyn2Types.AttributeValueMemberM{Value: m}, nil
}

func encodeValue(v interface{}) (dyn2Types.AttributeValue, error) {
	var e encoder
	if err := driver.Encode(reflect.ValueOf(v), &e); err != nil {
		return nil, err
	}
	return e.av, nil
}

func encodeFloat(f float64) dyn2Types.AttributeValue {
	return &dyn2Types.AttributeValueMemberN{Value: strconv.FormatFloat(f, 'f', -1, 64)}
}

////////////////////////////////////////////////////////////////

func decodeDoc(item dyn2Types.AttributeValue, doc driver.Document) error {
	return doc.Decode(decoder{av: item})
}

type decoder struct {
	av dyn2Types.AttributeValue
}

func (d decoder) String() string {
	if s, ok := d.av.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprint(d.av)
}

func (d decoder) AsBool() (bool, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return i.Value, true
}

func (d decoder) AsNull() bool {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberNULL)
	if !ok {
		return false
	}
	return i.Value
}

func (d decoder) AsString() (string, bool) {
	// Empty string is represented by NULL.
	_, ok := d.av.(*dyn2Types.AttributeValueMemberNULL)
	if ok {
		return "", true
	}
	i, ok := d.av.(*dyn2Types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return i.Value, true
}

func (d decoder) AsInt() (int64, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberN)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(i.Value, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (d decoder) AsUint() (uint64, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberN)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(i.Value, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (d decoder) AsFloat() (float64, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberN)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(i.Value, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func (d decoder) AsComplex() (complex128, bool) {
	iface, ok := d.av.(*dyn2Types.AttributeValueMemberL)
	if !ok {
		return 0, false
	}
	if len(iface.Value) != 2 {
		return 0, false
	}
	r, ok := decoder{iface.Value[0]}.AsFloat()
	if !ok {
		return 0, false
	}
	i, ok := decoder{iface.Value[1]}.AsFloat()
	if !ok {
		return 0, false
	}
	return complex(r, i), true
}

func (d decoder) AsBytes() ([]byte, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberB)
	if !ok {
		return nil, false
	}
	return i.Value, true
}

func (d decoder) ListLen() (int, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberL)
	if !ok {
		return 0, false
	}
	return len(i.Value), true
}

func (d decoder) DecodeList(f func(i int, vd driver.Decoder) bool) {
	iface, ok := d.av.(*dyn2Types.AttributeValueMemberL)
	if !ok {
		// V1 behavior treated this as indistinct from an empty list,
		// which this return replicates.
		// Should this be explicitly handled in some way?
		return
	}
	for i, el := range iface.Value {
		if !f(i, decoder{el}) {
			break
		}
	}
}

func (d decoder) MapLen() (int, bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberM)
	if !ok {
		return 0, false
	}
	return len(i.Value), true
}

func (d decoder) DecodeMap(f func(key string, vd driver.Decoder, exactMatch bool) bool) {
	i, ok := d.av.(*dyn2Types.AttributeValueMemberM)
	if !ok {
		// V1 behavior treated this as indistinct from an empty map,
		// which this return replicates.
		// Should this be explicitly handled in some way?
		return
	}
	for k, av := range i.Value {
		if !f(k, decoder{av}, true) {
			break
		}
	}
}

func (d decoder) AsInterface() (interface{}, error) {
	return toGoValue(d.av)
}

func toGoValue(av dyn2Types.AttributeValue) (interface{}, error) {
	switch v := av.(type) {
	case *dyn2Types.AttributeValueMemberNULL:
		return nil, nil
	case *dyn2Types.AttributeValueMemberBOOL:
		return v.Value, nil
	case *dyn2Types.AttributeValueMemberN:
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return nil, err
		}
		i := int64(f)
		if float64(i) == f {
			return i, nil
		}
		u := uint64(f)
		if float64(u) == f {
			return u, nil
		}
		return f, nil

	case *dyn2Types.AttributeValueMemberB:
		return v.Value, nil
	case *dyn2Types.AttributeValueMemberS:
		return v.Value, nil

	case *dyn2Types.AttributeValueMemberL:
		s := make([]interface{}, len(v.Value))
		for i, v := range v.Value {
			x, err := toGoValue(v)
			if err != nil {
				return nil, err
			}
			s[i] = x
		}
		return s, nil

	case *dyn2Types.AttributeValueMemberM:
		m := make(map[string]interface{}, len(v.Value))
		for k, v := range v.Value {
			x, err := toGoValue(v)
			if err != nil {
				return nil, err
			}
			m[k] = x
		}
		return m, nil

	default:
		return nil, fmt.Errorf("awsdynamodb: AttributeValue %s not supported", av)
	}
}

func (d decoder) AsSpecial(v reflect.Value) (bool, interface{}, error) {
	unsupportedTypes := `unsupported type, the docstore driver for DynamoDB does
	not decode DynamoDB set types, such as string set, number set and binary set`
	switch d.av.(type) {
	case *dyn2Types.AttributeValueMemberSS:
		return true, nil, errors.New(unsupportedTypes)
	case *dyn2Types.AttributeValueMemberNS:
		return true, nil, errors.New(unsupportedTypes)
	case *dyn2Types.AttributeValueMemberBS:
		return true, nil, errors.New(unsupportedTypes)
	}

	switch v.Type() {
	case typeOfGoTime:
		i, ok := d.av.(*dyn2Types.AttributeValueMemberS)
		if !ok {
			return false, nil, errors.New("expected string field for time.Time")
		}
		t, err := time.Parse(time.RFC3339Nano, i.Value)
		return true, t, err
	}
	return false, nil, nil
}
