package setup

import (
	"os"
	"fmt"
	"time"
	"context"
	"flag"
	"net"
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

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
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
		DropRequestHeaders:  gfeDroppedHeaders,
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
	if mode == recorder.ModeRecording {
		// Add credentials for real RPCs.
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, grpc.WithTransportCredentials(grpccreds.NewClientTLSFromCert(nil, "")))
		opts = append(opts, grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: gcp.CredentialsTokenSource(creds)}))
	} else {
		// Establish a local listener for Dial to connect to and update endPoint
		// to point to it.
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		endPoint = l.Addr().String()
		opts = append(opts, grpc.WithInsecure())
	}
	conn, err := grpc.DialContext(ctx, endPoint, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return conn, done
}


// NewAzureTestPipeline creates a new connection for testing against Azure Blob.
// It requires setting environment variables for the Storage Account Name (AZURE_STORAGE_ACCOUNT_NAME) and a storage key (AZURE_STORAGE_ACCOUNT_KEY)
func NewAzureTestPipeline(ctx context.Context, t *testing.T) (pipeline pipeline.Pipeline, done func(), accountName string, accountKey string) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}	
	r, done, err := replay.NewRecorder(t, mode, &replay.ProviderMatcher{}, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}
	
	accountName = "gocloud"	
	accountKey = os.Getenv("AZURE_STORAGE_ACCOUNT_KEY")
	credentials, _ := azblob.NewSharedKeyCredential(accountName, accountKey)

	if !*Record {					
		accountKey = "FAKE_KEY"		
	}

	p := newPipeline(credentials, r)
	return p, done, accountName, accountKey
}

func newPipeline(c azblob.Credential, r *recorder.Recorder) pipeline.Pipeline {
	if c == nil {
		panic("pipeline credential can't be nil")
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

	return pipeline.NewPipeline(f, pipeline.Options{HTTPSender: newDefaultHTTPClientFactory(AzureHTTPClient(r)), Log: log})
}

func newDefaultHTTPClientFactory(pipelineHTTPClient *http.Client) pipeline.Factory {
	return pipeline.FactoryFunc(func(next pipeline.Policy, po *pipeline.PolicyOptions) pipeline.PolicyFunc {
		return func(ctx context.Context, request pipeline.Request) (pipeline.Response, error) {
			r, err := pipelineHTTPClient.Do(request.WithContext(ctx))
			if err != nil {
				err = pipeline.NewError(err, "HTTP request failed")
			}
			return pipeline.NewHTTPResponse(r), err
		}
	})
}
// AzureHTTPClient returns a new http.Client configured for Azure
func AzureHTTPClient(r *recorder.Recorder) *http.Client {
	if r != nil {
		return &http.Client{Transport: r}
	} else {
		return &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				Dial: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
					DualStack: true,
				}).Dial,
				MaxIdleConns:           0,
				MaxIdleConnsPerHost:    1000,
				IdleConnTimeout:        180 * time.Second,
				TLSHandshakeTimeout:    10 * time.Second,
				ExpectContinueTimeout:  1 * time.Second,
				DisableKeepAlives:      false,
				DisableCompression:     false,
				MaxResponseHeaderBytes: 0,
			},
		}
	}
}

