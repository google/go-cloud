// Copyright 2025 The Go Cloud Development Kit Authors
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

package otel

import (
	"testing"
)

type testDriver struct{}

func TestProviderName(t *testing.T) {
	testCases := []struct {
		name   string
		driver any
		want   string
	}{
		{"nil", nil, ""},
		{"struct", testDriver{}, "gocloud.dev/internal/otel"},
		{"pointer", &testDriver{}, "gocloud.dev/internal/otel"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProviderName(tc.driver)
			if got != tc.want {
				t.Errorf("ProviderName(%#v) = %q, want %q", tc.driver, got, tc.want)
			}
		})
	}
}
