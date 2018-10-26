package setup

import (
	"context"
	"flag"
	"net/http"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"

	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

var Record = flag.Bool("record", false, "whether to run tests against cloud resources and record the interactions")

// NewAWSSession creates a new session for testing against AWS.
// If the test is in --record mode, the test will call out to AWS, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewAWSSession(t *testing.T, region string) (sess *session.Session, rt http.RoundTripper, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}
	awsMatcher := &replay.ProviderMatcher{
		URLScrubbers: []*regexp.Regexp{
			regexp.MustCompile(`X-Amz-(Credential|Signature)=[^?]*`),
		},
		Headers: []string{"X-Amz-Target"},
	}
	r, done, err := replay.NewRecorder(t, mode, awsMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	client := &http.Client{Transport: r}

	// Provide fake creds if running in replay mode.
	var creds *awscreds.Credentials
	if !*Record {
		creds = awscreds.NewStaticCredentials("FAKE_ID", "FAKE_SECRET", "FAKE_TOKEN")
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

	return sess, r, done
}

// NewGCPClient creates a new HTTPClient for testing against GCP.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewGCPClient(ctx context.Context, t *testing.T) (client *gcp.HTTPClient, rt http.RoundTripper, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	// GFEs scrub X-Google- and X-GFE- headers from requests and responses.
	// Drop them from recordings made by users inside Google.
	// http://g3doc/gfe/g3doc/gfe3/design/http_filters/google_header_filter
	// (internal Google documentation).
	gfeDroppedHeaders := regexp.MustCompile("^X-(Google|GFE)-")

	gcpMatcher := &replay.ProviderMatcher{
		DropRequestHeaders: gfeDroppedHeaders,
		DropResponseHeaders: gfeDroppedHeaders,
		URLScrubbers: []*regexp.Regexp{
			regexp.MustCompile(`Expires=[^?]*`),
		},
		BodyScrubbers: []*regexp.Regexp{regexp.MustCompile(`(?m)^\s*--.*$`)},
	}
	r, done, err := replay.NewRecorder(t, mode, gcpMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	if *Record {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatalf("failed to get default credentials: %v", err)
		}
		client, err = gcp.NewHTTPClient(r, gcp.CredentialsTokenSource(creds))
		if err != nil {
			t.Fatal(err)
		}
	} else {
		client = &gcp.HTTPClient{Client: http.Client{Transport: r}}
	}
	return client, r, done
}

// NewGCPgRPCConn creates a new connection for testing against GCP via gRPC.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing RPC and uses fake credentials.
func NewGCPgRPCConn(ctx context.Context, t *testing.T, endPoint string) (*grpc.ClientConn, func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	opts, done := replay.NewGCPDialOptions(t, mode, t.Name()+".replay")
	opts = append(opts, grpc.WithTransportCredentials(grpccreds.NewClientTLSFromCert(nil, "")))
	if mode == recorder.ModeRecording {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: gcp.CredentialsTokenSource(creds)}))
	}
	conn, err := grpc.DialContext(ctx, endPoint, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return conn, done
}
