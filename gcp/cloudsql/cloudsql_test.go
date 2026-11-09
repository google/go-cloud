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

package cloudsql_test

import (
	"testing"

	"gocloud.dev/gcp/cloudsql"
)

func TestParseIPType(t *testing.T) {
	tests := []struct {
		input   string
		want    cloudsql.IPType
		wantErr bool
	}{
		{"auto", cloudsql.IPTypeAuto, false},
		{"public", cloudsql.IPTypePublic, false},
		{"private", cloudsql.IPTypePrivate, false},
		{"psc", cloudsql.IPTypePSC, false},
		{"", cloudsql.IPTypeAuto, false},         // empty string defaults to auto
		{"PUBLIC", cloudsql.IPTypePublic, false}, // case-insensitive
		{"invalid", 0, true},
		{"public ", 0, true}, // whitespace is not trimmed
	}
	for _, tc := range tests {
		got, err := cloudsql.ParseIPType(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseIPType(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseIPType(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
