// Copyright 2021 The Go Cloud Development Kit Authors
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

// +build !linux

package fileblob

import "os"

var (
	// Disable support for openat2 syscalls on this OS
	// to skip opening the directory in OpenBucket.
	openat2Supported int32 = 0
)

func (*bucket) osOpen(name string) (*os.File, error) {
	return os.Open(name)
}

// initFeatures examines OS support to toggle feature flags.
func (b *bucket) initFeatures() error {
	return nil
}

func (b *bucket) Close() error {
	return nil
}
