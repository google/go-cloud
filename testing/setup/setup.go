package setup

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/testing/replay"
)

func NewAWSSession(t *testing.T, region, filename string) (sess *session.Session, done func()) {
	mode := recorder.ModeRecording
	if testing.Short() {
		mode = recorder.ModeReplaying
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
	if testing.Short() {
		creds = credentials.NewStaticCredentials("FAKE_ID", "FAKE_SECRET", "FAKE_TOKEN")
	}

	sess, err = session.NewSession(&aws.Config{
		HTTPClient:  client,
		Region:      aws.String(region),
		Credentials: creds,
	})
	if err != nil {
		t.Fatal(err)
	}

	return sess, done
}
