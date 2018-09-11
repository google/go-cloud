package setup

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"

	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/2018-03-28/azblob"
	
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
	}

	client := &http.Client{
		Transport: r,
	}

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
	return client, done
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


// NewAzureTestPipeline creates a new connection for testing against Azure Blob.
// It requires setting environment variables for the Storage Account Name (ACCOUNT_NAME) and a storage key (ACCOUNT_KEY)
func NewAzureTestPipeline(ctx context.Context, t *testing.T) (pipeline pipeline.Pipeline, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	azMatcher := &replay.ProviderMatcher{
		Headers: []string{"X-Az-Target"},
	}

	r, done, err := replay.NewRecorder(t, mode, azMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	accountName := os.Getenv("ACCOUNT_NAME")
	accountKey := os.Getenv("ACCOUNT_KEY")
	credentials := azblob.NewSharedKeyCredential(accountName, accountKey)

	p := newPipeline(credentials, r)
	return p, done
}

func newPipeline(c azblob.Credential, r *recorder.Recorder) pipeline.Pipeline {
	if c == nil {
		panic("c can't be nil")
	}

	f := []pipeline.Factory{

		azblob.NewTelemetryPolicyFactory(azblob.TelemetryOptions{
			Value: "X-Az-Target",
		}),
		azblob.NewUniqueRequestIDPolicyFactory(),
		azblob.NewRetryPolicyFactory(azblob.RetryOptions{
			Policy:        azblob.RetryPolicyExponential,
			MaxTries:      3,
			TryTimeout:    3 * time.Second,
			RetryDelay:    1 * time.Second,
			MaxRetryDelay: 3 * time.Second,
		}),
	}

	f = append(f, c)
	f = append(f, pipeline.MethodFactoryMarker())

	log := pipeline.LogOptions{
		Log: func(level pipeline.LogLevel, message string) {
			fmt.Println(message)
		},
		ShouldLog: func(level pipeline.LogLevel) bool {
			return true
		},
	}

	return pipeline.NewPipeline(f, pipeline.Options{HTTPSender: newDefaultHTTPClientFactory(r), Log: log})
}

func newDefaultHTTPClientFactory(r *recorder.Recorder) pipeline.Factory {
	return pipeline.FactoryFunc(func(next pipeline.Policy, po *pipeline.PolicyOptions) pipeline.PolicyFunc {
		return func(ctx context.Context, request pipeline.Request) (pipeline.Response, error) {
			pipelineHTTPClient := azureHTTPClient(r)
			r, err := pipelineHTTPClient.Do(request.WithContext(ctx))
			if err != nil {
				panic(err)
			}
			return pipeline.NewHTTPResponse(r), err
		}
	})
}

func azureHTTPClient(r *recorder.Recorder) *http.Client {
	if r != nil {
		return &http.Client{Transport: r}
	}
	// We want the Transport to have a large connection pool
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			// We use Dial instead of DialContext as DialContext has been reported to cause slower performance.
			Dial /*Context*/ : (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).Dial, /*Context*/
			MaxIdleConns:           0, // No limit
			MaxIdleConnsPerHost:    100,
			IdleConnTimeout:        90 * time.Second,
			TLSHandshakeTimeout:    10 * time.Second,
			ExpectContinueTimeout:  1 * time.Second,
			DisableKeepAlives:      false,
			DisableCompression:     false,
			MaxResponseHeaderBytes: 0,
		},
	}
}
