// These tests utilize a recorder to replay AWS endpoint hits from golden files.
// Golden files are used if -short is passed to `go test`.
// If -short is not passed, the recorder will make a call to AWS and save a new golden file.
package paramstore

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
)

const region = "us-east-2"

// scrub removes *potentially* sensitive information from a golden file.
// The golden file must only have a single interaction.
// TODO(cflewis): Investigate possible ways to ensure this runs after
// every test, otherwise a mistake could cause a golden file to
// be pushed to review and become globally visible.
func scrub(filepath string) error {
	c, err := cassette.Load(filepath)
	if err != nil {
		return fmt.Errorf("unable to load golden file, do not commit to repository: %v", err)
	}

	// Always get the interaction that's going to get scrubbed.
	c.Matcher = func(_ *http.Request, _ cassette.Request) bool { return true }
	interaction, err := c.GetInteraction(nil)
	if err != nil {
		return fmt.Errorf("unable to load interaction, do not commit to repository: %v", err)
	}

	interaction.Request.Headers.Del("Authorization")
	interaction.Response.Headers.Del("X-Amzn-Requestid")
	c.Save()

	return nil
}

func TestRead(t *testing.T) {
	path := filepath.Join("testdata", "read_test")
	t.Logf("Golden file is at %v", path)

	mode := recorder.ModeReplaying
	if !testing.Short() {
		t.Logf("Recording into golden file")
		mode = recorder.ModeRecording
	}
	r, err := recorder.NewAsMode(path, mode, nil)
	if err != nil {
		log.Fatalf("unable to record: %v", err)
	}
	defer func() { r.Stop(); scrub(path) }()

	client := &http.Client{
		Transport: r,
	}

	sess, err := session.NewSession(aws.NewConfig().WithHTTPClient(client).WithRegion(region))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "Good path should return the parameter",
			path: "/",
			want: "cflewis-string-test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, err := Read(sess, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case len(params) == 0:
				t.Error("got 0 params, want 1")
			case *params[0].Name != tc.want:
				t.Errorf("want %v; got %v", tc.want, *params[0].Name)
			}
		})
	}
}
