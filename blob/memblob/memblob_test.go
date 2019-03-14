// Copyright 2018 The Go Cloud Development Kit Authors
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

package memblob

import (
	"context"
	"net/http"
	"testing"

	"gocloud.dev/blob"
	"gocloud.dev/blob/driver"
	"gocloud.dev/blob/drivertest"
)

type harness struct{}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return &harness{}, nil
}

func (h *harness) HTTPClient() *http.Client {
	return nil
}

func (h *harness) MakeDriver(ctx context.Context) (driver.Bucket, error) {
	return openBucket(nil), nil
}

func (h *harness) Close() {}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, nil)
}

func BenchmarkMemblob(b *testing.B) {
	drivertest.RunBenchmarks(b, OpenBucket(nil))
}

func TestOpenBucketFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"mem://", false},
		// Invalid parameter.
		{"mem://?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := blob.OpenBucket(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
