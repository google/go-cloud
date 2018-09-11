package azureblob

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"context"
	"flag"
	"os"
	"testing"

	"github.com/Azure/azure-storage-blob-go/2018-03-28/azblob"
	"github.com/dnaeon/go-vcr/recorder"

	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/drivertest"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/google/go-cloud/internal/testing/replay"
)

const (
	bucketName   = "go-cloud-bucket"
	headerTarget = "X-Az-Target"
)

var Record = flag.Bool("record", true, "whether to run tests against cloud resources and record the interactions")

type harness struct {
	settings Settings
	closer   func()
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	settings, done := NewAzureTestSettings(ctx, t)

	return &harness{settings: settings, closer: done}, nil
}

func (h *harness) MakeBucket(ctx context.Context) (*blob.Bucket, error) {
	return OpenBucket(ctx, &h.settings, bucketName)
}

func (h *harness) Close() {
	h.closer()
}
func TestConformance(t *testing.T) {
	// test requires the following environment variables
	// Account_Name is the Azure Storage Account Name
	// Account_Key is the Primary or Secondary Key
	os.Setenv("ACCOUNT_NAME", "gocloud")
	os.Setenv("ACCOUNT_KEY", "")

	drivertest.RunConformanceTests(t, newHarness, "../testdata")
}

func NewAzureTestSettings(ctx context.Context, t *testing.T) (settings Settings, done func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	azMatcher := &replay.ProviderMatcher{
		Headers: []string{headerTarget},
	}

	r, done, err := replay.NewRecorder(t, mode, azMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	accountName := os.Getenv("ACCOUNT_NAME")
	accountKey := os.Getenv("ACCOUNT_KEY")
	credentials := azblob.NewSharedKeyCredential(accountName, accountKey)

	p := newPipeline(credentials, r)
	s := Settings{
		AccountName:      accountName,
		AccountKey:       accountKey,
		PublicAccessType: PublicAccessBlob,
		SASToken:         "",
		Pipeline:         p,
	}

	return s, done
}

func newPipeline(c azblob.Credential, r *recorder.Recorder) pipeline.Pipeline {
	if c == nil {
		panic("c can't be nil")
	}

	f := []pipeline.Factory{

		azblob.NewTelemetryPolicyFactory(azblob.TelemetryOptions{
			Value: headerTarget,
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
