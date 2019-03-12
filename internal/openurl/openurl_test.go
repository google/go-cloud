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
		url     string
		wantErr bool
		want    interface{}
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
		if _, _, err := emptyM.FromString("type", test.url); err == nil {
			t.Errorf("%s: empty SchemeMap got nil error, wanted non-nil error", test.url)
		}

		got, gotURL, gotErr := m.FromString("type", test.url)
		if (gotErr != nil) != test.wantErr {
			t.Errorf("%s: got error %v, want error: %v", test.url, gotErr, test.wantErr)
		}
		if gotErr != nil {
			continue
		}
		if got := gotURL.String(); got != test.url {
			t.Errorf("%s: got URL %q want %v", test.url, got, test.url)
		}
		if got != test.want {
			t.Errorf("%s: got %v want %v", test.url, got, test.want)
		}
	}

}
