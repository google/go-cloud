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
	"path/filepath"
	"regexp"
	"strconv"
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
	// we inspect the bodies as well.
	last := -1
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
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

		// TODO(rvangent): Make this more consistent between providers.
		switch provider {
		case ProviderGCP:
			body = scrubGCSIDs(body)
			url = scrubGCSURL(url)
		case ProviderAWS:
		default:
			t.Fatalf("Unsupported Provider: %v", provider)
		}

		// TODO(rvangent): See if we can get rid of these X-Gocloud-Seq headers.
		seq, _ := strconv.Atoi(i.Headers.Get("X-Gocloud-Seq"))

		t.Logf("%s: Targets: %v | %v == %v\n", path, r.Header.Get("X-Amz-Target"), i.Headers.Get("X-Amz-Target"), r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target"))
		t.Logf("%s: URLs: %v | %v == %v\n", path, url, i.URL, url == i.URL)
		t.Logf("%s: Methods: %v | %v == %v\n", path, r.Method, i.Method, r.Method == i.Method)
		t.Logf("%s: Bodies:\n%v\n~~~~~~~~~~~\n%v\n==%v\n", path, body, i.Body, body == i.Body)
		t.Logf("%s: Sequences: %v | %v == %v\n", path, seq, last, seq > last)

		// TODO(rvangent): See if we need X-Amz-Target, and if so, only use it for
		// ProviderGCP.
		if r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target") &&
			url == i.URL &&
			r.Method == i.Method &&
			body == i.Body &&
			seq > last {
			last = seq
			t.Logf("%s: returning header match for seq %d", path, seq)
			return true
		}

		t.Logf("%s: no match, continuing...", path)
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

// scrubAWSHeaders removes *potentially* sensitive information from a cassette,
// and adds sequencing to the requests to differentiate Amazon calls, as they
// aren't timestamped.
// Note that sequence numbers should only be used for otherwise identical matches.
func scrubAWSHeaders(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load golden file, do not commit to repository: %v", err)
	}

	c.Mu.Lock()
	for i, action := range c.Interactions {
		action.Request.Headers.Set("X-Gocloud-Seq", strconv.Itoa(i))
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
