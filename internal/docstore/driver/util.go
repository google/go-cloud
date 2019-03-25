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
	"github.com/google/uuid"
)

// UniqueString generates a string that is unique with high probability.
// Driver implementations can use it to generate keys for Create actions.
func UniqueString() string { return uuid.New().String() }

// SplitActions divides the actions slice into sub-slices much like strings.Split.
// The split function should report whether two consecutive actions should be split,
// that is, should be in different sub-slices. The first argument to split is the
// last action of the sub-slice currently under construction; the second argument is
// the action being considered for addition to that sub-slice.
// SplitActions doesn't change the order of the input slice.
func SplitActions(actions []*Action, split func(a, b *Action) bool) [][]*Action {
	var (
		groups [][]*Action // the actions, split; the return value
		cur    []*Action   // the group currently being constructed
	)
	collect := func() { // called when the current group is known to be finished
		if len(cur) > 0 {
			groups = append(groups, cur)
			cur = nil
		}
	}
	for _, a := range actions {
		if len(cur) > 0 && split(cur[len(cur)-1], a) {
			collect()
		}
		cur = append(cur, a)
	}
	collect()
	return groups
}
