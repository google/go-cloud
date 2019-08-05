// Copyright 2019 The Go Cloud Development Kit Authors
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

package setup // import "gocloud.dev/internal/testing/setup"

import (
	"context"
	"flag"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/useragent"

	"github.com/google/go-replayers/grpcreplay"
	"github.com/google/go-replayers/httpreplay"
	hrgoog "github.com/google/go-replayers/httpreplay/google"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
)

// Record is true iff the tests are being run in "record" mode.
var Record = flag.Bool("record", false, "whether to run tests against cloud resources and record the interactions")

// FakeGCPCredentials gets fake GCP credentials.
func FakeGCPCredentials(ctx context.Context) (*google.Credentials, error) {
	return google.CredentialsFromJSON(ctx, []byte(`{"type": "service_account", "project_id": "my-project-id"}`))
}

func awsSession(region string, client *http.Client) (*session.Session, error) {
	// Provide fake creds if running in replay mode.
	var creds *awscreds.Credentials
	if !*Record {
		creds = awscreds.NewStaticCredentials("FAKE_ID", "FAKE_SECRET", "FAKE_TOKEN")
	}
	return session.NewSession(&aws.Config{
		HTTPClient:  client,
		Region:      aws.String(region),
		Credentials: creds,
		MaxRetries:  aws.Int(0),
	})
}

// NewRecordReplayClient creates a new http.Client for tests. This client's
// activity is being either recorded to files (when *Record is set) or replayed
// from files. rf is a modifier function that will be invoked with the address
// of the httpreplay.Recorder object used to obtain the client; this function
// can mutate the recorder to add service-specific header filters, for example.
// An initState is returned for tests that need a state to have deterministic
// results, for example, a seed to generate random sequences.
func NewRecordReplayClient(ctx context.Context, t *testing.T, rf func(r *httpreplay.Recorder)) (c *http.Client, cleanup func(), initState int64) {
	httpreplay.DebugHeaders()
	path := filepath.Join("testdata", t.Name()+".replay")
	if *Record {
		t.Logf("Recording into golden file %s", path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		state := time.Now()
		b, _ := state.MarshalBinary()
		rec, err := httpreplay.NewRecorder(path, b)
		if err != nil {
			t.Fatal(err)
		}
		rf(rec)
		cleanup = func() {
			if err := rec.Close(); err != nil {
				t.Fatal(err)
			}
		}

		return rec.Client(), cleanup, state.UnixNano()
	}
	t.Logf("Replaying from golden file %s", path)
	rep, err := httpreplay.NewReplayer(path)
	if err != nil {
		t.Fatal(err)
	}
	recState := new(time.Time)
	if err := recState.UnmarshalBinary(rep.Initial()); err != nil {
		t.Fatal(err)
	}
	return rep.Client(), func() { rep.Close() }, recState.UnixNano()
}

// NewAWSSession creates a new session for testing against AWS.
// If the test is in --record mode, the test will call out to AWS, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
// An initState is returned for tests that need a state to have deterministic
// results, for example, a seed to generate random sequences.
func NewAWSSession(ctx context.Context, t *testing.T, region string) (sess *session.Session,
	rt http.RoundTripper, cleanup func(), initState int64) {
	client, cleanup, state := NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		r.RemoveQueryParams("X-Amz-Credential", "X-Amz-Signature", "X-Amz-Security-Token")
		r.RemoveRequestHeaders("Authorization", "Duration", "X-Amz-Security-Token")
		r.ClearHeaders("X-Amz-Date")
		r.ClearQueryParams("X-Amz-Date")
		r.ClearHeaders("User-Agent") // AWS includes the Go version
	})
	sess, err := awsSession(region, client)
	if err != nil {
		t.Fatal(err)
	}
	return sess, client.Transport, cleanup, state
}

// NewGCPClient creates a new HTTPClient for testing against GCP.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewGCPClient(ctx context.Context, t *testing.T) (client *gcp.HTTPClient, rt http.RoundTripper, done func()) {
	c, cleanup, _ := NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		r.ClearQueryParams("Expires")
		r.ClearQueryParams("Signature")
		r.ClearHeaders("Expires")
		r.ClearHeaders("Signature")
	})
	transport := c.Transport
	if *Record {
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatalf("failed to get default credentials: %v", err)
		}
		c, err = hrgoog.RecordClient(ctx, c, option.WithTokenSource(gcp.CredentialsTokenSource(creds)))
		if err != nil {
			t.Fatal(err)
		}
	}
	return &gcp.HTTPClient{Client: *c}, transport, cleanup
}

// NewGCPgRPCConn creates a new connection for testing against GCP via gRPC.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing RPC and uses fake credentials.
func NewGCPgRPCConn(ctx context.Context, t *testing.T, endPoint, api string) (*grpc.ClientConn, func()) {
	filename := t.Name() + ".replay"
	if *Record {
		opts, done := newGCPRecordDialOptions(t, filename)
		opts = append(opts, useragent.GRPCDialOption(api))
		// Add credentials for real RPCs.
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, grpc.WithTransportCredentials(grpccreds.NewClientTLSFromCert(nil, "")))
		opts = append(opts, grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: gcp.CredentialsTokenSource(creds)}))
		conn, err := grpc.DialContext(ctx, endPoint, opts...)
		if err != nil {
			t.Fatal(err)
		}
		return conn, done
	}
	rep, done := newGCPReplayer(t, filename)
	conn, err := rep.Connection()
	if err != nil {
		t.Fatal(err)
	}
	return conn, done
}

// contentTypeInjectPolicy and contentTypeInjector are somewhat of a hack to
// overcome an impedance mismatch between the Azure pipeline library and
// httpreplay - the tool we use to record/replay HTTP traffic for tests.
// azure-pipeline-go does not set the Content-Type header in its requests,
// setting X-Ms-Blob-Content-Type instead; however, httpreplay expects
// Content-Type to be non-empty in some cases. This injector makes sure that
// the content type is copied into the right header when that is originally
// empty. It's only used for testing.
type contentTypeInjectPolicy struct {
	node pipeline.Policy
}

func (p *contentTypeInjectPolicy) Do(ctx context.Context, request pipeline.Request) (pipeline.Response, error) {
	if len(request.Header.Get("Content-Type")) == 0 {
		cType := request.Header.Get("X-Ms-Blob-Content-Type")
		request.Header.Set("Content-Type", cType)
	}
	response, err := p.node.Do(ctx, request)
	return response, err
}

type contentTypeInjector struct {
}

func (f contentTypeInjector) New(node pipeline.Policy, opts *pipeline.PolicyOptions) pipeline.Policy {
	return &contentTypeInjectPolicy{node: node}
}

// NewAzureTestPipeline creates a new connection for testing against Azure Blob.
func NewAzureTestPipeline(ctx context.Context, t *testing.T, api string, credential azblob.Credential, accountName string) (pipeline.Pipeline, func(), *http.Client) {
	client, done, _ := NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		r.RemoveQueryParams("se", "sig")
		r.RemoveQueryParams("X-Ms-Date")
		r.ClearHeaders("X-Ms-Date")
		r.ClearHeaders("User-Agent") // includes the full Go version
	})
	f := []pipeline.Factory{
		// Sets User-Agent for recorder.
		azblob.NewTelemetryPolicyFactory(azblob.TelemetryOptions{
			Value: useragent.AzureUserAgentPrefix(api),
		}),
		contentTypeInjector{},
		credential,
		pipeline.MethodFactoryMarker(),
	}
	// Create a pipeline that uses client to make requests.
	p := pipeline.NewPipeline(f, pipeline.Options{
		HTTPSender: pipeline.FactoryFunc(func(next pipeline.Policy, po *pipeline.PolicyOptions) pipeline.PolicyFunc {
			return func(ctx context.Context, request pipeline.Request) (pipeline.Response, error) {
				r, err := client.Do(request.WithContext(ctx))
				if err != nil {
					err = pipeline.NewError(err, "HTTP request failed")
				}
				return pipeline.NewHTTPResponse(r), err
			}
		}),
	})

	return p, done, client
}

// NewAzureKeyVaultTestClient creates a *http.Client for Azure KeyVault test
// recordings.
func NewAzureKeyVaultTestClient(ctx context.Context, t *testing.T) (*http.Client, func()) {
	client, cleanup, _ := NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		r.RemoveQueryParams("se", "sig")
		r.RemoveQueryParams("X-Ms-Date")
		r.ClearHeaders("X-Ms-Date")
		r.ClearHeaders("User-Agent") // includes the full Go version
	})
	return client, cleanup
}

// FakeGCPDefaultCredentials sets up the environment with fake GCP credentials.
// It returns a cleanup function.
func FakeGCPDefaultCredentials(t *testing.T) func() {
	const envVar = "GOOGLE_APPLICATION_CREDENTIALS"
	jsonCred := []byte(`{"client_id": "foo.apps.googleusercontent.com", "client_secret": "bar", "refresh_token": "baz", "type": "authorized_user"}`)
	f, err := ioutil.TempFile("", "fake-gcp-creds")
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(f.Name(), jsonCred, 0666); err != nil {
		t.Fatal(err)
	}
	oldEnvVal := os.Getenv(envVar)
	os.Setenv(envVar, f.Name())
	return func() {
		os.Remove(f.Name())
		os.Setenv(envVar, oldEnvVal)
	}
}

// newGCPRecordDialOptions return grpc.DialOptions that are to be appended to a
// GRPC dial request. These options allow a recorder to intercept RPCs and save
// RPCs to the file at filename, or read the RPCs from the file and return them.
func newGCPRecordDialOptions(t *testing.T, filename string) (opts []grpc.DialOption, done func()) {
	path := filepath.Join("testdata", filename)
	t.Logf("Recording into golden file %s", path)
	r, err := grpcreplay.NewRecorder(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	opts = r.DialOptions()
	done = func() {
		if err := r.Close(); err != nil {
			t.Errorf("unable to close recorder: %v", err)
		}
	}
	return opts, done
}

// newGCPReplayer returns a Replayer for GCP gRPC connections, as well as a function
// to call when done with the Replayer.
func newGCPReplayer(t *testing.T, filename string) (*grpcreplay.Replayer, func()) {
	path := filepath.Join("testdata", filename)
	t.Logf("Replaying from golden file %s", path)
	r, err := grpcreplay.NewReplayer(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := func() {
		if err := r.Close(); err != nil {
			t.Errorf("unable to close recorder: %v", err)
		}
	}
	return r, done
}

// HasDockerTestEnvironment returns true when either:
// 1) Not on Travis.
// 2) On Travis Linux environment, where Docker is available.
func HasDockerTestEnvironment() bool {
	s := os.Getenv("TRAVIS_OS_NAME")
	return s == "" || s == "linux"
}
