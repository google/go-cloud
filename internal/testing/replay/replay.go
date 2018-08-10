// Copyright 2018 Google LLC
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
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"cloud.google.com/go/rpcreplay"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/golang/protobuf/proto"
	"github.com/tidwall/sjson"
	"google.golang.org/grpc"
)

// Provider represents a cloud provider that we're going to record/replay
// HTTP requests with.
type Provider int

const (
	ProviderAWS Provider = iota
	ProviderGCP
)

// NewRecorder returns a go-vcr.Recorder which reads or writes golden files from the given path.
// The recorder uses matching and scrubbing algorithms specific to the provider.
// The done() function flushes the recorder, scrubs the data, and saves it to the golden file.
// The golden file is not written to disk if the done() function is not called.
func NewRecorder(t *testing.T, provider Provider, mode recorder.Mode, filename string) (r *recorder.Recorder, done func(), err error) {
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

	// Use a custom matcher as the default matcher looks for URLs and methods;
	// we inspect the bodies, some headers, and also prevent re-matches.
	cur := 0
	logged := false
	used := map[int]bool{}
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		// If we've already used the request at this index, skip it.
		if used[cur] {
			cur++
			return false
		}

		// Gather info about the request we're looking for, and scrub it.
		var b bytes.Buffer
		if r.Body != nil {
			if _, err := b.ReadFrom(r.Body); err != nil {
				t.Logf(err.Error())
				return false
			}
		}
		r.Body = ioutil.NopCloser(&b)
		body := b.String()
		url := r.URL.String()
		target, iTarget := "", ""

		// TODO(rvangent): Make this more consistent between providers.
		switch provider {
		case ProviderGCP:
			body = scrubGCSIDs(body)
			url = scrubGCSURL(url)
		case ProviderAWS:
			target = r.Header.Get("X-Amz-Target")
			iTarget = i.Headers.Get("X-Amz-Target")
		default:
			t.Fatalf("Unsupported Provider: %v", provider)
		}

		// Log info about the request we're looking for.
		if !logged {
			t.Logf("%s: Looking for:\n  Method: %s\n  URL: %s\n  Target: %s\n  Body: %s", path, r.Method, url, target, body)
			logged = true
		}

		if r.Method == i.Method && url == i.URL && target == iTarget && body == i.Body {
			t.Logf("%s: found match at index %d", path, cur)
			used[cur] = true
			cur = 0
			logged = false
			return true
		}
		cur++
		return false
	})
	return r, func() {
		if err := r.Stop(); err != nil {
			fmt.Println(err)
		}
		if mode != recorder.ModeRecording {
			return
		}
		switch provider {
		case ProviderGCP:
			if err := scrubGCS(path); err != nil {
				fmt.Println(err)
			}
		case ProviderAWS:
			if err := scrubAWSHeaders(path); err != nil {
				fmt.Println(err)
			}
		default:
			t.Fatalf("Unsupported Provider: %v", provider)
		}
	}, nil
}

// scrubAWSHeaders removes *potentially* sensitive information from a cassette.
func scrubAWSHeaders(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load golden file, do not commit to repository: %v", err)
	}

	c.Mu.Lock()
	for _, action := range c.Interactions {
		action.Request.Headers.Del("Authorization")
		action.Response.Headers.Del("X-Amzn-Requestid")
		// Ignore an error if LastModifiedUser isn't deleted, it's not really important.
		action.Response.Body, _ = sjson.Delete(action.Response.Body, "LastModifiedUser")
	}
	c.Mu.Unlock()
	return c.Save()
}

// NewGCPDialOptions return grpc.DialOptions that are to be appended to a GRPC dial request.
// These options allow a recorder/replayer to intercept RPCs and save RPCs to the file at filename,
// or read the RPCs from the file and return them.
func NewGCPDialOptions(logf func(string, ...interface{}), mode recorder.Mode, filename string, scrubber func(func(string, ...interface{}), string, proto.Message) error) (opts []grpc.DialOption, done func(), err error) {
	path := filepath.Join("testdata", filename)
	logf("Golden file is at %v", path)

	if mode == recorder.ModeRecording {
		logf("Recording into golden file")
		r, err := rpcreplay.NewRecorder(path, nil)
		if err != nil {
			return nil, nil, err
		}
		r.BeforeFunc = func(s string, m proto.Message) error {
			return scrubber(logf, s, m)
		}
		opts = r.DialOptions()
		done = func() {
			if err := r.Close(); err != nil {
				logf("unable to close recorder: %v", err)
			}
		}
		return opts, done, nil
	}

	logf("Replaying from golden file")
	r, err := rpcreplay.NewReplayer(path)
	if err != nil {
		return nil, nil, err
	}
	r.BeforeFunc = func(s string, m proto.Message) error {
		return scrubber(logf, s, m)
	}
	opts = r.DialOptions()
	done = func() {
		if err := r.Close(); err != nil {
			logf("unable to close replayer: %v", err)
		}
	}

	return opts, done, nil
}

func scrubGCSURL(url string) string {
	return strings.Split(url, "&")[0]
}

func scrubGCSIDs(body string) string {
	re := regexp.MustCompile(`(?m)^\s*--.*$`)
	return re.ReplaceAllString(body, "")
}

func scrubGCSResponse(body string) string {
	if strings.Contains(body, `"debugInfo"`) {
		return ""
	}

	for _, v := range []string{"selfLink", "md5Hash", "generation", "acl", "mediaLink", "owner"} {
		// Ignore errors, as they'll contain issues like the key not existing, which is fine.
		// This is a best effort scrub and the golden files should be code reviewed
		// anyway.
		body, _ = sjson.Delete(body, v)
	}

	return body
}

// scrubGCS scrubs both the headers and body for sensitive information, and massages the
// GCS request/response bodies to remove unique identifiers that prevent body matching.
func scrubGCS(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load golden file, do not commit to repository: %v", err)
	}

	c.Mu.Lock()
	for _, action := range c.Interactions {
		action.Request.URL = scrubGCSURL(action.Request.URL)
		action.Request.Headers.Del("Authorization")
		action.Request.Body = scrubGCSIDs(action.Request.Body)
		action.Response.Body = scrubGCSResponse(action.Response.Body)

		for header, _ := range action.Response.Headers {
			if strings.HasPrefix(header, "X-Google") || strings.HasPrefix(header, "X-Guploader") {
				action.Response.Headers.Del(header)
			}
		}
	}
	c.Mu.Unlock()
	return c.Save()
}
