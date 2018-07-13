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
	"strconv"
	"strings"

	"cloud.google.com/go/rpcreplay"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/golang/protobuf/proto"
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
	// AWS inspects the bodies as well.
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
		if err := scrubAWSHeaders(path); err != nil {
			fmt.Println(err)
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
		action.Response.Body = removeJSONString(action.Response.Body, "LastModifiedUser")
	}
	c.Mu.Unlock()
	c.Save()

	return nil
}

// removeJSONString removes a stringed value from a JSON string.
// This is very dodgy and doesn't work if the value has a " in it,
// or if the key is the last in the list, or probably other things
// too. If the case isn't tested, it probably doesn't work.
// json.Decoder.Token could be used to make this way better.
func removeJSONString(s, key string) string {
	re := regexp.MustCompile(`(?U)\"` + key + `\":\s?\".*\",`)
	s = strings.Replace(s, "\n", "", -1)
	return re.ReplaceAllString(s, "")
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

func NewGCSRecorder(logf func(string, ...interface{}), mode recorder.Mode, filename string) (r *recorder.Recorder, done func(), err error) {
	path := filepath.Join("testdata", filename)
	logf("Golden file is at %v", path)

	if mode == recorder.ModeRecording {
		logf("Recording into golden file")
	} else {
		logf("Replaying from golden file")
	}

	r, err = recorder.NewAsMode(path, mode, nil)
	if err != nil {
		return nil, nil, err
	}

	// Use a custom matcher as the default matcher looks for URLs and methods.
	// GCS inspects the bodies as well.
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		var b bytes.Buffer
		if r.Body != nil {
			if _, err := b.ReadFrom(r.Body); err != nil {
				logf(err.Error())
				return false
			}
		}
		r.Body = ioutil.NopCloser(&b)

		logf("URLs: %v | %v == %v\n", r.URL.String(), i.URL, r.URL.String() == i.URL)
		logf("Methods: %v | %v == %v\n", r.Method, i.Method, r.Method == i.Method)
		logf("Bodies:\n%v\n%v\n==%v\n", b.String(), i.Body, b.String() == i.Body)

		if scrubGCSURL(r.URL.String()) == i.URL &&
			r.Method == i.Method &&
			scrubGCSBody(b.String()) == i.Body {
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
		if err := scrubGCSHeaders(path); err != nil {
			fmt.Println(err)
		}
	}, nil
}

func scrubGCSURL(url string) string {
	return strings.Split(url, "&")[0]
}

func scrubGCSBody(body string) string {
	if strings.Contains(body, `"debugInfo"`) {
		return ""
	}

	return body
}

// scrubGCSHeaders removes *potentially* sensitive information from a cassette.
func scrubGCSHeaders(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load golden file, do not commit to repository: %v", err)
	}

	c.Mu.Lock()
	for _, action := range c.Interactions {
		action.Request.URL = scrubGCSURL(action.Request.URL)
		action.Request.Headers.Del("Authorization")
		action.Response.Body = scrubGCSBody(action.Response.Body)
		action.Response.Body = removeJSONString(action.Response.Body, "selfLink")
		action.Response.Body = removeJSONString(action.Response.Body, "name")
		action.Response.Body = removeJSONString(action.Response.Body, "id")
		action.Response.Body = removeJSONString(action.Response.Body, "projectNumber")

		for header, _ := range action.Response.Headers {
			if strings.HasPrefix(header, "X-Google") || strings.HasPrefix(header, "X-Guploader") {
				action.Response.Headers.Del(header)
			}
		}
	}
	c.Mu.Unlock()
	c.Save()

	return nil
}
