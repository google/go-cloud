// Copyright 2026 The Go Cloud Development Kit Authors
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

package huaweiblob

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// subresourceState is the in-memory backing store of the httptest OBS server
// used by the versioning/lifecycle tests below.
type subresourceState struct {
	mu         sync.Mutex
	versioning map[string]string // bucket -> status
	lifecycle  map[string][]byte // bucket -> raw XML configuration
}

// newSubresourceServer starts a local OBS-compatible server that implements
// only the ?versioning and ?lifecycle sub-resources, and returns a driver
// bucket pointed at it (path-style, via the http:// endpoint prefix).
func newSubresourceServer(t *testing.T) (*bucket, *subresourceState, func()) {
	t.Helper()
	st := &subresourceState{
		versioning: map[string]string{},
		lifecycle:  map[string][]byte{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		bucketName := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)[0]
		q := r.URL.Query()
		switch {
		case q.Has("versioning"):
			switch r.Method {
			case http.MethodPut:
				body, _ := io.ReadAll(r.Body)
				var cfg VersioningConfiguration
				if err := xml.Unmarshal(body, &cfg); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				st.versioning[bucketName] = cfg.Status
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				body, _ := xml.Marshal(VersioningConfiguration{Status: st.versioning[bucketName]})
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write(body)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case q.Has("lifecycle"):
			switch r.Method {
			case http.MethodPut:
				body, _ := io.ReadAll(r.Body)
				st.lifecycle[bucketName] = body
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				body, ok := st.lifecycle[bucketName]
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("<Error><Code>NoSuchLifecycleConfiguration</Code><Message>missing</Message></Error>"))
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				_, _ = w.Write(body)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	drv, err := openBucket(context.Background(), &Options{
		Endpoint:    srv.URL,
		BucketName:  "subresource-test-bucket",
		AccessKeyID: "FAKE_ACCESS_KEY",
		SecretKey:   "FAKE_SECRET_KEY",
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		srv.Close()
		t.Fatalf("openBucket: %v", err)
	}
	return drv.(*bucket), st, srv.Close
}

func TestEnableAndGetBucketVersioning(t *testing.T) {
	ctx := context.Background()
	b, st, cleanup := newSubresourceServer(t)
	defer cleanup()

	// Never configured: blank status, and the server must have been hit.
	cfg, err := b.GetBucketVersioning(ctx)
	if err != nil {
		t.Fatalf("GetBucketVersioning (unset): %v", err)
	}
	if cfg.Status != "" {
		t.Errorf("unset versioning: got status %q, want blank", cfg.Status)
	}

	// Invalid statuses are rejected client-side (no server round-trip).
	for _, bad := range []string{"", "enabled", "Bogus"} {
		if err := b.EnableBucketVersioning(ctx, bad); err == nil {
			t.Errorf("EnableBucketVersioning(%q): got nil error, want invalid-status error", bad)
		}
	}

	// Enabled round-trips through the server.
	if err := b.EnableBucketVersioning(ctx, VersioningStatusEnabled); err != nil {
		t.Fatalf("EnableBucketVersioning(Enabled): %v", err)
	}
	if got := st.versioning["subresource-test-bucket"]; got != "Enabled" {
		t.Fatalf("server state after Enable: got %q, want Enabled", got)
	}
	cfg, err = b.GetBucketVersioning(ctx)
	if err != nil {
		t.Fatalf("GetBucketVersioning (enabled): %v", err)
	}
	if cfg.Status != VersioningStatusEnabled {
		t.Errorf("got status %q, want %q", cfg.Status, VersioningStatusEnabled)
	}

	// Suspended replaces Enabled.
	if err := b.EnableBucketVersioning(ctx, VersioningStatusSuspended); err != nil {
		t.Fatalf("EnableBucketVersioning(Suspended): %v", err)
	}
	cfg, err = b.GetBucketVersioning(ctx)
	if err != nil {
		t.Fatalf("GetBucketVersioning (suspended): %v", err)
	}
	if cfg.Status != VersioningStatusSuspended {
		t.Errorf("got status %q, want %q", cfg.Status, VersioningStatusSuspended)
	}
}

func TestPutAndGetLifecycleConfiguration(t *testing.T) {
	ctx := context.Background()
	b, _, cleanup := newSubresourceServer(t)
	defer cleanup()

	// No configuration yet: (nil, nil), not an error.
	got, err := b.GetLifecycleConfiguration(ctx)
	if err != nil {
		t.Fatalf("GetLifecycleConfiguration (unset): %v", err)
	}
	if got != nil {
		t.Errorf("unset lifecycle: got %+v, want nil", got)
	}

	// Invalid configurations are rejected client-side.
	if err := b.PutLifecycleConfiguration(ctx, nil); err == nil {
		t.Error("PutLifecycleConfiguration(nil): got nil error, want error")
	}
	if err := b.PutLifecycleConfiguration(ctx, &LifecycleConfiguration{}); err == nil {
		t.Error("PutLifecycleConfiguration(no rules): got nil error, want error")
	}
	if err := b.PutLifecycleConfiguration(ctx, &LifecycleConfiguration{
		Rules: []LifecycleRule{{ID: "bad", Status: "Bogus"}},
	}); err == nil {
		t.Error("PutLifecycleConfiguration(invalid status): got nil error, want error")
	}

	// A full rule round-trips with every field preserved.
	want := &LifecycleConfiguration{
		Rules: []LifecycleRule{
			{
				ID:     "rule-1",
				Status: LifecycleRuleStatusEnabled,
				Filter: &LifecycleRuleFilter{Prefix: "logs/"},
				Transition: &LifecycleTransition{
					Days:         10,
					StorageClass: "WARM",
				},
				Expiration:                  &LifecycleExpiration{Days: 30},
				NoncurrentVersionExpiration: &LifecycleNoncurrentVersionExpiration{NoncurrentDays: 7},
				AbortIncompleteMultipartUpload: &LifecycleAbortIncompleteMultipartUpload{
					DaysAfterInitiation: 3,
				},
			},
			{
				ID:         "rule-2",
				Status:     LifecycleRuleStatusDisabled,
				Expiration: &LifecycleExpiration{Days: 365},
			},
		},
	}
	if err := b.PutLifecycleConfiguration(ctx, want); err != nil {
		t.Fatalf("PutLifecycleConfiguration: %v", err)
	}
	got, err = b.GetLifecycleConfiguration(ctx)
	if err != nil {
		t.Fatalf("GetLifecycleConfiguration: %v", err)
	}
	// Compare the rules payload; XMLName is populated by xml.Unmarshal on the
	// decoded value but left zero on the constructed want, so it is not part
	// of the semantic round-trip check.
	if !reflect.DeepEqual(got.Rules, want.Rules) {
		t.Errorf("lifecycle round-trip mismatch:\ngot  %+v\nwant %+v", got.Rules, want.Rules)
	}
}
