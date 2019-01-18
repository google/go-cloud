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

package pubsub

import (
	"testing"
	"time"
)

func TestAverager(t *testing.T) {
	a := newAverager(time.Minute, 4)
	start := time.Now()
	for i := 0; i < 10; i++ {
		a.addInternal(5, start)
	}
	if got, want := a.average(), 5.0; got != want {
		t.Errorf("got %f, want %f", got, want)
	}

	a = newAverager(time.Minute, 4)
	start = time.Now()
	n := 60
	var total float64
	for i := 0; i < n; i++ {
		total += float64(i)
		a.addInternal(float64(i), start.Add(time.Duration(i)*time.Second))
	}
	// All the points are within one minute of each other, so they should all be counted.
	got := a.average()
	want := total / float64(n)
	if got != want {
		t.Errorf("got %f, want %f", got, want)
	}

	// Values older than the duration are dropped.
	a = newAverager(time.Minute, 4)
	a.add(10)
	if got, want := a.average(), 10.0; got != want {
		t.Errorf("got %f, want %f", got, want)
	}
	a.addInternal(3, a.end.Add(2*time.Minute))
	if got, want := a.average(), 3.0; got != want {
		t.Errorf("got %f, want %f", got, want)
	}
}
