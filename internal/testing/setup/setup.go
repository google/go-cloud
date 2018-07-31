package setup

import (
	"flag"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/internal/testing/replay"
)

var Record = flag.Bool("record", false, "whether to run tests against cloud resources and record the interactions")

// NewAWSSession creates a new session for testing against AWS.
// If the test is short, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
// If the test is not short, the test will call out to AWS, and the results recorded
// as a new replay file.
func NewAWSSession(t *testing.T, region, filename string) (sess *session.Session, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}
	r, done, err := replay.NewAWSRecorder(t.Logf, mode, filename)
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	client := &http.Client{
		Transport: r,
	}

	// Provide fake creds if running in replay mode.
	var creds *credentials.Credentials
	if !*Record {
		creds = credentials.NewStaticCredentials("FAKE_ID", "FAKE_SECRET", "FAKE_TOKEN")
	}

	sess, err = session.NewSession(&aws.Config{
		HTTPClient:  client,
		Region:      aws.String(region),
		Credentials: creds,
		MaxRetries:  aws.Int(0),
	})
	if err != nil {
		t.Fatal(err)
	}

	return sess, done
}
