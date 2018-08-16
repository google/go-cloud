package setup

import (
	"context"
	"flag"
	"net/http"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"
)

var Record = flag.Bool("record", false, "whether to run tests against cloud resources and record the interactions")

// NewAWSSession creates a new session for testing against AWS.
// If the test is in --record mode, the test will call out to AWS, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewAWSSession(t *testing.T, region string) (sess *session.Session, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}
	awsMatcher := &replay.ProviderMatcher{
		Headers: []string{"X-Amz-Target"},
	}
	r, done, err := replay.NewRecorder(t, mode, awsMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
		return nil, nil
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

// NewGCPClient creates a new HTTPClient for testing against GCP.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewGCPClient(ctx context.Context, t *testing.T) (client *gcp.HTTPClient, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	gcpMatcher := &replay.ProviderMatcher{
		BodyScrubbers: []*regexp.Regexp{regexp.MustCompile(`(?m)^\s*--.*$`)},
	}
	r, done, err := replay.NewRecorder(t, mode, gcpMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
		return nil, nil
	}

	if *Record {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatalf("failed to get default credentials: %v", err)
			return nil, nil
		}
		client, err = gcp.NewHTTPClient(r, gcp.CredentialsTokenSource(creds))
		if err != nil {
			t.Fatal(err)
			return nil, nil
		}
	} else {
		client = &gcp.HTTPClient{Client: http.Client{Transport: r}}
	}
	return client, done
}
