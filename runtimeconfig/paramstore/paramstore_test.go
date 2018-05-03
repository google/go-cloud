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

// These tests utilize a recorder to replay AWS endpoint hits from golden files.
// Golden files are used if -short is passed to `go test`.
// If -short is not passed, the recorder will make a call to AWS and save a new golden file.
package paramstore

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
)

const region = "us-east-2"

// TestWriteReadDelete attempts to write, read and then delete parameters from Parameter Store.
// This test can't be broken up into separate Test(Write|Read|Delete) tests
// because the only way to make the test hermetic is for the test to be able
// to perform all the functions.
func TestWriteReadDelete(t *testing.T) {
	tests := []struct {
		name, param string
		wantErr     bool
	}{
		{
			name:  "Good param should pass",
			param: "write-read-delete-test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sess, done := newSession(t, "write_read_delete")
			defer done()
			_, err := writeParam(sess, tc.param, "String", "Snowman: ☃️")
			if err != nil {
				t.Fatal(err)
			}

			p, err := readParam(sess, tc.param, -1)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case p.name != tc.param:
				t.Errorf("want %s; got %s", tc.param, p.name)
			}

			if deleteParam(sess, tc.param) != nil {
				t.Error(err)
			}
		})
	}
}

func TestInitialWatch(t *testing.T) {
	tests := []struct {
		name, param, value string
		wantErr            bool
	}{
		{
			name:  "Good param should return OK",
			param: "watch-initial-test",
			value: "foobar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sess, done := newSession(t, "watch_initial")
			defer done()

			if _, err := writeParam(sess, tc.param, "String", tc.value); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := deleteParam(sess, tc.param); err != nil {
					t.Fatal(err)
				}
			}()

			ctx := context.Background()
			conf, err := NewClient(ctx, sess).NewConfig(ctx, tc.param, &WatchOptions{WaitTime: time.Second})
			res, err := conf.Watch(ctx)

			switch {
			case err != nil:
				t.Fatal(err)
			case res.Value != tc.value:
				t.Errorf("want %v; got %v", tc.value, res.Value)
			}
		})
	}
}

func TestWatchObservesChange(t *testing.T) {
	tests := []struct {
		name, param, firstValue, secondValue string
		wantErr                              bool
	}{
		{
			name:        "Good param should flip OK",
			param:       "watch-observes-change-test",
			firstValue:  "foo",
			secondValue: "bar",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sess, done := newSession(t, "watch_change")
			defer done()

			if _, err := writeParam(sess, tc.param, "String", tc.firstValue); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := deleteParam(sess, tc.param); err != nil {
					t.Fatal(err)
				}
			}()

			ctx := context.Background()
			conf, err := NewClient(ctx, sess).NewConfig(ctx, tc.param, &WatchOptions{WaitTime: time.Second})
			res, err := conf.Watch(ctx)
			switch {
			case err != nil:
				t.Fatal(err)
			case res.Value != tc.firstValue:
				t.Errorf("want %v; got %v", tc.firstValue, res.Value)
			}

			// Write again and see that watch sees the new value.
			if _, err := writeParam(sess, tc.param, "String", tc.secondValue); err != nil {
				t.Fatal(err)
			}

			res, err = conf.Watch(ctx)
			switch {
			case err != nil:
				t.Fatal(err)
			case res.Value != tc.secondValue:
				t.Errorf("want %v; got %v", tc.secondValue, res.Value)
			}
		})
	}
}

func newRecorder(t *testing.T, filename string) (r *recorder.Recorder, done func()) {
	path := filepath.Join("testdata", filename)
	t.Logf("Golden file is at %v", path)

	mode := recorder.ModeReplaying
	if !testing.Short() {
		t.Logf("Recording into golden file")
		mode = recorder.ModeRecording
	}
	r, err := recorder.NewAsMode(path, mode, nil)
	if err != nil {
		t.Fatalf("unable to record: %v", err)
	}

	// Use a custom matcher as the default matcher looks for URLs and methods,
	// which Amazon overloads as it isn't RESTful.
	// Sequencing is added to the requests when the cassette is saved, which
	// allows for differentiating GETs which otherwise look identical.
	last := -1
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		if r.Body == nil {
			return false
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(r.Body); err != nil {
			t.Fatal(err)
		}
		r.Body = ioutil.NopCloser(&b)

		seq, err := strconv.Atoi(i.Headers.Get("X-Gocloud-Seq"))
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("Targets: %v | %v == %v\n", r.Header.Get("X-Amz-Target"), i.Headers.Get("X-Amz-Target"), r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target"))
		t.Logf("URLs: %v | %v == %v\n", r.URL.String(), i.URL, r.URL.String() == i.URL)
		t.Logf("Methods: %v | %v == %v\n", r.Method, i.Method, r.Method == i.Method)
		t.Logf("Bodies:\n%v\n%v\n==%v\n", b.String(), i.Body, b.String() == i.Body)

		if r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target") &&
			r.URL.String() == i.URL &&
			r.Method == i.Method &&
			b.String() == i.Body &&
			seq > last {
			last = seq
			return true
		}

		return false
	})

	return r, func() { r.Stop(); fixHeaders(path) }
}

func newSession(t *testing.T, filename string) (sess *session.Session, done func()) {
	r, done := newRecorder(t, filename)
	defer done()

	client := &http.Client{
		Transport: r,
	}

	// Provide fake creds if running in replay mode.
	var creds *credentials.Credentials
	if testing.Short() {
		creds = credentials.NewStaticCredentials("FAKE_ID", "FAKE_SECRET", "FAKE_TOKEN")
	}

	sess, err := session.NewSession(aws.NewConfig().WithHTTPClient(client).WithRegion(region).WithCredentials(creds))
	if err != nil {
		t.Fatal(err)
	}

	return sess, done
}

// fixHeaders removes *potentially* sensitive information from a cassette,
// and adds sequencing to the requests to differentiate Amazon calls, as they
// aren't timestamped.
// Note that sequence numbers should only be used for otherwise identical matches.
func fixHeaders(filepath string) error {
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

func writeParam(p client.ConfigProvider, name, pType, value string) (int64, error) {
	svc := ssm.New(p)
	resp, err := svc.PutParameter(&ssm.PutParameterInput{
		Name:      aws.String(name),
		Type:      aws.String(pType),
		Value:     aws.String(value),
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return -1, err
	}

	return *resp.Version, err
}

func deleteParam(p client.ConfigProvider, name string) error {
	svc := ssm.New(p)
	_, err := svc.DeleteParameter(&ssm.DeleteParameterInput{Name: aws.String(name)})
	return err
}
