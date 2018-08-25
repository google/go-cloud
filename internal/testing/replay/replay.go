// Copyright 2018 The Go Cloud Authors
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

// Package replay provides the ability to record and replay HTTP requests.
package replay

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"cloud.google.com/go/rpcreplay"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"google.golang.org/grpc"
)

type ProviderMatcher struct {
	// Headers is a slice of HTTP request headers that will be verified to match.
	Headers []string
	// BodyScrubber is a slice of regular expressions that will be used to
	// scrub the HTTP request body before matching, via ReplaceAllString.
	BodyScrubbers []*regexp.Regexp
}

// NewRecorder returns a go-vcr.Recorder which reads or writes golden files from the given path.
// When recording, done() saves the recording to a golden file. The
// Authorization request header is dropped, but otherwise the raw HTTP
// requests/responses are saved
// When replaying, HTTP requests are expected to arrive in the same order as
// in the recording. They are verified to have the same:
// -- Method
// -- URL
// -- Specific HTTP headers ((optional, via matcher).
// -- Body (optionally scrubbed, via matcher).
func NewRecorder(t *testing.T, mode recorder.Mode, matcher *ProviderMatcher, filename string) (r *recorder.Recorder, done func(), err error) {
	path := filepath.Join("testdata", filename)
	if mode == recorder.ModeRecording {
		t.Logf("Recording into golden file %s", path)
	} else {
		t.Logf("Replaying from golden file %s", path)
	}
	r, err = recorder.NewAsMode(path, mode, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to record: %v", err)
	}

	// Use a custom matcher as the default matcher looks for URLs and methods.
	// We match strictly in order, and verify URL, method, (possibly scrubbed)
	// body, and (possibly) some headers.
	cur := 0
	lastMatch := -1
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		// If we've already used the request at this index, skip it.
		if cur <= lastMatch {
			cur++
			return false
		}

		if r.Method != i.Method {
			t.Fatalf("mismatched Method at request #%d; got %q want %q", cur, r.Method, i.Method)
		}
		if r.URL.String() != i.URL {
			t.Fatalf("mismatched URL at request #%d; got %q want %q", cur, r.URL, i.URL)
		}
		for _, header := range matcher.Headers {
			got := r.Header.Get(header)
			want := i.Headers.Get(header)
			if got != want {
				t.Fatalf("mismatched HTTP header %q header at request #%d; got %q want %q", header, cur, got, want)
			}
		}
		var b bytes.Buffer
		if r.Body != nil {
			if _, err := b.ReadFrom(r.Body); err != nil {
				t.Fatalf("couldn't read request body: %v", err)
			}
		}
		r.Body = ioutil.NopCloser(&b)
		gotBody := b.String()
		wantBody := i.Body
		for _, re := range matcher.BodyScrubbers {
			gotBody = re.ReplaceAllString(gotBody, "")
			wantBody = re.ReplaceAllString(wantBody, "")
		}

		// We've got a match!
		t.Logf("matched request #%d (%s %s)", cur, i.Method, i.URL)
		lastMatch = cur
		cur = 0
		return true
	})
	return r, func() {
		if err := r.Stop(); err != nil {
			fmt.Println(err)
		}
		if mode == recorder.ModeRecording {
			if err := scrubRecording(path); err != nil {
				t.Errorf("failed to scrub recording: %v", err)
			}
		}
	}, nil
}

// scrubRecording scrubs the Authorization header.
func scrubRecording(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing to scrub!
			return nil
		}
		return err
	}

	keep := make([]*cassette.Interaction, 0, len(c.Interactions))
	c.Mu.Lock()
	for _, action := range c.Interactions {
		if action.Response.Code == http.StatusTooManyRequests {
			// Too Many Requests, client will retry; it's safe
			// to drop these for the replay.
			continue
		}
		action.Request.Headers.Del("Authorization")
		action.Response.Duration = ""
		keep = append(keep, action)
	}
	c.Interactions = keep
	c.Mu.Unlock()
	return c.Save()
}

// NewGCPDialOptions return grpc.DialOptions that are to be appended to a GRPC dial request.
// These options allow a recorder/replayer to intercept RPCs and save RPCs to the file at filename,
// or read the RPCs from the file and return them.
func NewGCPDialOptions(t *testing.T, mode recorder.Mode, filename string) (opts []grpc.DialOption, done func()) {
	path := filepath.Join("testdata", filename)
	if mode == recorder.ModeRecording {
		t.Logf("Recording into golden file %s", path)
		r, err := rpcreplay.NewRecorder(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		opts = r.DialOptions()
		done = func() {
			if err := r.Close(); err != nil {
				t.Errorf("unable to close recorder: %v", err)
			}
		}
		return opts, done
	}
	t.Logf("Replaying from golden file %s", path)
	r, err := rpcreplay.NewReplayer(path)
	if err != nil {
		t.Fatal(err)
	}
	opts = r.DialOptions()
	done = func() {
		if err := r.Close(); err != nil {
			t.Errorf("unable to close recorder: %v", err)
		}
	}
	return opts, done
}
