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

// Useful comparison functions.

package driver

import (
	"fmt"
	"math/big"
	"reflect"
	"time"
)

// CompareTimes returns -1, 1 or 0 depending on whether t1 is before, after or
// equal to t2.
func CompareTimes(t1, t2 time.Time) int {
	switch {
	case t1.Before(t2):
		return -1
	case t1.After(t2):
		return 1
	default:
		return 0
	}
}

// CompareNumbers returns -1, 1 or 0 depending on whether n1 is less than,
// greater than or equal to n2. n1 and n2 must be signed integer, unsigned
// integer, or floating-point values, but they need not be the same type.
//
// If both types are integers or both floating-point, CompareNumbers behaves
// like Go comparisons on those types. If one operand is integer and the other
// is floating-point, CompareNumbers correctly compares the mathematical values
// of the numbers, without loss of precision.
func CompareNumbers(n1, n2 interface{}) (int, error) {
	v1, ok := n1.(reflect.Value)
	if !ok {
		v1 = reflect.ValueOf(n1)
	}
	v2, ok := n2.(reflect.Value)
	if !ok {
		v2 = reflect.ValueOf(n2)
	}
	f1, err := toBigFloat(v1)
	if err != nil {
		return 0, err
	}
	f2, err := toBigFloat(v2)
	if err != nil {
		return 0, err
	}
	return f1.Cmp(f2), nil
}

func toBigFloat(x reflect.Value) (*big.Float, error) {
	var f big.Float
	switch x.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f.SetInt64(x.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		f.SetUint64(x.Uint())
	case reflect.Float32, reflect.Float64:
		f.SetFloat64(x.Float())
	default:
		typ := "nil"
		if x.IsValid() {
			typ = fmt.Sprint(x.Type())
		}
		return nil, fmt.Errorf("%v of type %s is not a number", x, typ)
	}
	return &f, nil
}
