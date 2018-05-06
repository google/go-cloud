// These tests utilize a recorder to replay AWS endpoint hits from golden files.
// Golden files are used if -short is passed to `go test`.
// If -short is not passed, the recorder will make a call to AWS and save a new golden file.
package paramstore

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

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
		name, path, param string
		wantErr           bool
	}{
		{
			name:  "Good path and param should pass",
			path:  "/",
			param: "paramstore-string-test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sess, done := newSession(t, "write_read_delete")
			defer done()
			_, err := writeParam(sess, tc.param, tc.name, "String", "foobar")
			if err != nil {
				t.Fatal(err)
			}

			params, err := ReadParam(sess, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case len(params) == 0:
				t.Error("got 0 params, want 1")
			case *params[0].Name != tc.param:
				t.Errorf("want %v; got %v", tc.param, *params[0].Name)
			}

			if deleteParam(sess, tc.param) != nil {
				t.Error(err)
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
	// which Amazon overloads as it isn't RESTful. Match against the Target
	// instead.
	r.SetMatcher(func(r *http.Request, i cassette.Request) bool {
		return r.Header.Get("X-Amz-Target") == i.Headers.Get("X-Amz-Target") &&
			r.URL.String() == i.URL &&
			r.Method == i.Method
	})

	return r, func() { r.Stop(); scrub(path) }
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

// scrub removes *potentially* sensitive information from a golden file.
// The golden file must only have a single interaction.
func scrub(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load golden file, do not commit to repository: %v", err)
	}

	c.Mu.Lock()
	for _, i := range c.Interactions {
		i.Request.Headers.Del("Authorization")
		i.Response.Headers.Del("X-Amzn-Requestid")
	}
	c.Mu.Unlock()
	c.Save()

	return nil
}

func writeParam(p client.ConfigProvider, name, desc, pType, value string) (int64, error) {
	svc := ssm.New(p)
	resp, err := svc.PutParameter(&ssm.PutParameterInput{
		Name:        aws.String(name),
		Description: aws.String(desc),
		Type:        aws.String(pType),
		Value:       aws.String(value),
		Overwrite:   aws.Bool(true),
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
