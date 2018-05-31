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
	"strconv"

	"cloud.google.com/go/rpcreplay"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"google.golang.org/grpc"
)

// NewAWSRecorder returns a go-vcr.Recorder which reads or writes golden files from the given path.
// The recorder uses a matching algorithm specialized to AWS HTTP API calls.
// The done() function flushes the recorder and scrubs any sensitive data from the saved golden file.
// The golden file is not written to disk if the done() function is not called.
func NewAWSRecorder(logf func(string, ...interface{}), mode recorder.Mode, filename string) (r *recorder.Recorder, done func(), err error) {
	path := filepath.Join("testdata", filename)
	logf("Golden file is at %v", path)

	if mode == recorder.ModeRecording {
		logf("Recording into golden file")
	} else {
		logf("Replaying from golden file")
	}
	r, err = recorder.NewAsMode(path, mode, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to record: %v", err)
	}

	// Use a custom matcher as the default matcher looks for URLs and methods,
	// which Amazon overloads as it isn't RESTful.
	// Sequencing is added to the requests when the cassette is saved, which
	// allows for differentiating GETs which otherwise look identical.
	last := -1
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		var b bytes.Buffer
		if r.Body != nil {
			if _, err := b.ReadFrom(r.Body); err != nil {
				logf(err.Error())
				return false
			}
		}
		r.Body = ioutil.NopCloser(&b)

		seq, err := strconv.Atoi(i.Headers.Get("X-Gocloud-Seq"))
		if err != nil {
			logf(err.Error())
			return false
		}

		logf("Targets: %v | %v == %v\n", r.Header.Get("X-Amz-Target"), i.Headers.Get("X-Amz-Target"), r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target"))
		logf("URLs: %v | %v == %v\n", r.URL.String(), i.URL, r.URL.String() == i.URL)
		logf("Methods: %v | %v == %v\n", r.Method, i.Method, r.Method == i.Method)
		logf("Bodies:\n%v\n%v\n==%v\n", b.String(), i.Body, b.String() == i.Body)
		logf("Sequences: %v | %v == %v\n", seq, last, seq > last)

		if r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target") &&
			r.URL.String() == i.URL &&
			r.Method == i.Method &&
			b.String() == i.Body &&
			seq > last {
			last = seq
			logf("Returning header match")
			return true
		}

		logf("No match, continuing...")
		return false
	})
	return r, func() {
		r.Stop()
		if mode != recorder.ModeRecording {
			return
		}
		if err := fixAWSHeaders(path); err != nil {
			fmt.Println(err)
		}
	}, nil
}

// fixAWSHeaders removes *potentially* sensitive information from a cassette,
// and adds sequencing to the requests to differentiate Amazon calls, as they
// aren't timestamped.
// Note that sequence numbers should only be used for otherwise identical matches.
func fixAWSHeaders(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load cassette, do not commit to repository: %v", err)
	}

	c.Mu.Lock()
	for i, action := range c.Interactions {
		action.Request.Headers.Set("X-Gocloud-Seq", strconv.Itoa(i))
		action.Request.Headers.Del("Authorization")
		action.Response.Headers.Del("X-Amzn-Requestid")
	}
	c.Mu.Unlock()
	c.Save()

	return nil
}

// NewGCPDialOptions return grpc.DialOptions that are to be appended to a GRPC dial request.
// These options allow a recorder/replayer to intercept RPCs and save RPCs to the file at filename,
// or read the RPCs from the file and return them.
func NewGCPDialOptions(logf func(string, ...interface{}), mode recorder.Mode, filename string) (opts []grpc.DialOption, done func(), err error) {
	path := filepath.Join("testdata", filename)
	logf("Golden file is at %v", path)

	if mode == recorder.ModeRecording {
		logf("Recording into golden file")
		r, err := rpcreplay.NewRecorder(path, nil)
		if err != nil {
			return nil, nil, err
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
	opts = r.DialOptions()
	done = func() {
		if err := r.Close(); err != nil {
			logf("unable to close replayer: %v", err)
		}
	}

	return opts, done, nil
}
