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

package pubsub

import (
	"sync"
	"time"
)

// An averager keeps track of an average value over a time interval.
// It does so by tracking the average over several sub-intervals.
type averager struct {
	mu             sync.Mutex
	buckets        []bucket // always nBuckets of these, last is most recent
	bucketInterval time.Duration
	end            time.Time // latest time we can currently record (end of the last bucket's interval)
}

type bucket struct {
	total float64 // total of points in the bucket
	count float64 // number of points in the bucket
}

// newAverager returns an averager that will average a value over dur,
// by splitting it into nBuckets sub-intervals.
func newAverager(dur time.Duration, nBuckets int) *averager {
	bi := dur / time.Duration(nBuckets)
	return &averager{
		buckets:        make([]bucket, nBuckets),
		bucketInterval: bi,
		end:            time.Now().Add(bi),
	}
}

// average returns the average value.
// It returns NaN if there are no recorded points.
func (a *averager) average() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	var total, n float64
	for _, b := range a.buckets {
		total += b.total
		n += b.count
	}
	return total / n
}

// add adds a point to the average at the present time.
func (a *averager) add(x float64) { a.addInternal(x, time.Now()) }

// addInternal adds a point x to the average, at time t.
// t should be time.Now() except during testing.
func (a *averager) addInternal(x float64, t time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Add new buckets and discard old ones until we can accommodate time t.
	for t.After(a.end) {
		a.buckets = append(a.buckets[1:], bucket{})
		a.end = a.end.Add(a.bucketInterval)
	}
	// We only support adding to the most recent bucket.
	if t.Before(a.end.Add(-a.bucketInterval)) {
		panic("time too early")
	}
	b := &a.buckets[len(a.buckets)-1]
	b.total += x
	b.count++
}
