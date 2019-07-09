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

package awsdynamodb

import (
	"net/url"
	"testing"

	gcaws "gocloud.dev/aws"
)

func TestProcessURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"dynamodb://docstore-test?partition_key=_kind", false},
		// OK.
		{"dynamodb://docstore-test?partition_key=_kind&sort_key=_id", false},
		// OK, overriding region.
		{"dynamodb://docstore-test?partition_key=_kind&region=" + region, false},
		// OK, allow_scans.
		{"dynamodb://docstore-test?partition_key=_kind&allow_scans=true" + region, false},
		// Passing revision field.
		{"dynamodb://docstore-test?partition_key=_kind&revision_field=123", false},
		// Unknown parameter.
		{"dynamodb://docstore-test?partition_key=_kind&param=value", true},
		// With path.
		{"dynamodb://docstore-test/subcoll?partition_key=_kind", true},
		// Missing partition_key.
		{"dynamodb://docstore-test?sort_key=_id", true},
	}

	sess, err := gcaws.NewDefaultSession()
	if err != nil {
		t.Fatal(err)
	}
	o := &URLOpener{ConfigProvider: sess}
	for _, test := range tests {
		u, err := url.Parse(test.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, _, _, err = o.processURL(u)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
