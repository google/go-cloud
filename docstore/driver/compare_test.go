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
	"math"
	"testing"
	"time"
)

func TestCompareTimes(t *testing.T) {
	t1 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(1)
	for _, test := range []struct {
		in1, in2 time.Time
		want     int
	}{
		{t1, t2, -1},
		{t2, t1, 1},
		{t1, t1, 0},
	} {
		got := CompareTimes(test.in1, test.in2)
		if got != test.want {
			t.Errorf("CompareTimes(%v, %v) == %d, want %d", test.in1, test.in2, got, test.want)
		}
	}
}

func TestCompareNumbers(t *testing.T) {
	check := func(n1, n2 interface{}, want int) {
		t.Helper()
		got, err := CompareNumbers(n1, n2)
		if err != nil {
			t.Fatalf("CompareNumbers(%T(%[1]v), %T(%[2]v)): %v", n1, n2, err)
		}
		if got != want {
			t.Errorf("CompareNumbers(%T(%[1]v), %T(%[2]v)) = %d, want %d", n1, n2, got, want)
		}
	}

	for _, test := range []struct {
		in1, in2 interface{}
		want     int
	}{
		// simple cases
		{1, 1, 0},
		{1, 2, -1},
		{1.0, 1.0, 0},
		{1.0, 2.0, -1},

		// mixed int types
		{int8(1), int64(1), 0},
		{int8(2), int64(1), 1},
		{uint(1), int(1), 0},
		{uint(2), int(1), 1},

		// mixed int and float
		{1, 1.0, 0},
		{1, 1.1, -1},
		{2, 1.1, 1},

		// large numbers
		{int64(math.MaxInt64), int64(math.MaxInt64), 0},
		{uint64(math.MaxUint64), uint64(math.MaxUint64), 0},
		{float64(math.MaxFloat64), float64(math.MaxFloat64), 0},
		{int64(math.MaxInt64), int64(math.MaxInt64 - 1), 1},
		{int64(math.MaxInt64), float64(math.MaxInt64 - 1), -1}, // float is bigger because it gets rounded up
		{int64(math.MaxInt64), uint64(math.MaxUint64), -1},

		// special floats
		{int64(math.MaxInt64), math.Inf(1), -1},
		{int64(math.MinInt64), math.Inf(-1), 1},
	} {
		check(test.in1, test.in2, test.want)
		if test.want != 0 {
			check(test.in2, test.in1, -test.want)
		}
	}
}
