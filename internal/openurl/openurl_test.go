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
package openurl_test

import (
	"testing"

	"gocloud.dev/internal/openurl"
)

func TestSchemeMap(t *testing.T) {
	const foo, bar = "foo value", "bar value"

	tests := []struct {
		URL     string
		WantErr bool
		Want    interface{}
	}{
		{"invalid url", true, nil},
		{"foo://a/b/c", false, foo},
		{"bar://a?p=v", false, bar},
	}

	var emptyM, m openurl.SchemeMap
	m.Register("api", "type", "foo", foo)
	m.Register("api", "type", "bar", bar)

	for _, test := range tests {
		// Empty SchemeMap should always return an error.
		if _, _, err := emptyM.FromString("api", "type", test.URL); err == nil {
			t.Errorf("%s: empty SchemeMap got nil error, wanted non-nil error", test.URL)
		}

		got, gotURL, gotErr := m.FromString("api", "type", test.URL)
		if (gotErr != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error: %v", test.URL, gotErr, test.WantErr)
		}
		if gotErr != nil {
			continue
		}
		if gotURL.String() != test.URL {
			t.Errorf("%s: got URL %q want %v", test.URL, gotURL.String(), test.URL)
		}
		if got != test.Want {
			t.Errorf("%s: got %v want %v", test.URL, got, test.Want)
		}
	}

}
