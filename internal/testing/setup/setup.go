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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/dnaeon/go-vcr/recorder"
	"gocloud.dev/gcp"
	"gocloud.dev/internal/testing/replay"
	"gocloud.dev/internal/useragent"

	"cloud.google.com/go/httpreplay"
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

// NewAWSSession creates a new session for testing against AWS.
// If the test is in --record mode, the test will call out to AWS, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewAWSSession(t *testing.T, region string) (sess *session.Session, rt http.RoundTripper, cleanup func()) {
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
	r, cleanup, err := replay.NewRecorder(t, mode, awsMatcher, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	client := &http.Client{Transport: r}
	sess, err = awsSession(region, client)
	if err != nil {
		t.Fatal(err)
	}
	return sess, r, cleanup
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

// NewAWSSession2 is like NewAWSSession, but it uses a different record/replay proxy.
func NewAWSSession2(ctx context.Context, t *testing.T, region string) (sess *session.Session, rt http.RoundTripper, cleanup func()) {
	httpreplay.DebugHeaders()
	path := filepath.Join("testdata", t.Name()+".replay")
	if *Record {
		t.Logf("Recording into golden file %s", path)
	} else {
		t.Logf("Replaying from golden file %s", path)
	}
	if *Record {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		rec, err := httpreplay.NewRecorder(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		rec.RemoveQueryParams("X-Amz-Credential", "X-Amz-Signature", "X-Amz-Security-Token")
		rec.RemoveRequestHeaders("Authorization", "Duration", "X-Amz-Security-Token")
		rec.ClearHeaders("X-Amz-Date")
		rec.ClearQueryParams("X-Amz-Date")
		rec.ClearHeaders("User-Agent") // AWS includes the Go version
		c, err := rec.Client(ctx, option.WithoutAuthentication())
		if err != nil {
			t.Fatal(err)
		}
		rt = c.Transport
		cleanup = func() {
			if err := rec.Close(); err != nil {
				t.Fatal(err)
			}
		}
	} else { // replay
		rep, err := httpreplay.NewReplayer(path)
		if err != nil {
			t.Fatal(err)
		}
		c, err := rep.Client(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rt = c.Transport
		cleanup = func() { _ = rep.Close() } // Don't care about Close error on replay.
	}
	sess, err := awsSession(region, &http.Client{Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	return sess, rt, cleanup
}

// NewGCPClient creates a new HTTPClient for testing against GCP.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
func NewGCPClient(ctx context.Context, t *testing.T) (client *gcp.HTTPClient, rt http.RoundTripper, done func()) {
	httpreplay.DebugHeaders()
	path := filepath.Join("testdata", t.Name()+".replay")

	if *Record {
		t.Logf("Recording into golden file %s", path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		rec, err := httpreplay.NewRecorder(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		rec.ClearQueryParams("Expires")
		rec.ClearQueryParams("Signature")
		rec.ClearHeaders("Expires")
		rec.ClearHeaders("Signature")
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatalf("failed to get default credentials: %v", err)
		}
		client, err := rec.Client(ctx, option.WithTokenSource(gcp.CredentialsTokenSource(creds)))
		if err != nil {
			t.Fatal(err)
		}
		cleanup := func() {
			if err := rec.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return &gcp.HTTPClient{Client: *client}, client.Transport, cleanup
	}

	// Replay.
	t.Logf("Replaying from golden file %s", path)
	replay, err := httpreplay.NewReplayer(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := replay.Client(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_ = replay.Close()
	}
	return &gcp.HTTPClient{Client: *c}, c.Transport, cleanup
}

// NewGCPgRPCConn creates a new connection for testing against GCP via gRPC.
// If the test is in --record mode, the client will call out to GCP, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing RPC and uses fake credentials.
func NewGCPgRPCConn(ctx context.Context, t *testing.T, endPoint, api string) (*grpc.ClientConn, func()) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	opts, done := replay.NewGCPDialOptions(t, mode, t.Name()+".replay")
	opts = append(opts, useragent.GRPCDialOption(api))
	if mode == recorder.ModeRecording {
		// Add credentials for real RPCs.
		creds, err := gcp.DefaultCredentials(ctx)
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, grpc.WithTransportCredentials(grpccreds.NewClientTLSFromCert(nil, "")))
		opts = append(opts, grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: gcp.CredentialsTokenSource(creds)}))
	} else {
		// Establish a local gRPC server for Dial to connect to and update endPoint
		// to point to it.
		// As of grpc 1.18, we must create a true gRPC server.
		srv := grpc.NewServer()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			if err := srv.Serve(l); err != nil {
				t.Error(err)
			}
		}()
		defer srv.Stop()
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
func NewAzureTestPipeline(ctx context.Context, t *testing.T, api string, credential azblob.Credential, accountName string) (pipeline.Pipeline, func(), *http.Client) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	azMatchers := &replay.ProviderMatcher{
		// Note: We can't match the User-Agent header because Azure includes the
		// "go" version in it.
		// Headers: []string{"User-Agent"},
		URLScrubbers: []*regexp.Regexp{
			regexp.MustCompile(`se=[^?]*`),
			regexp.MustCompile(`sig=[^?]*`),
		},
	}
	r, done, err := replay.NewRecorder(t, mode, azMatchers, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}
	f := []pipeline.Factory{
		// Sets User-Agent for recorder.
		azblob.NewTelemetryPolicyFactory(azblob.TelemetryOptions{
			Value: useragent.AzureUserAgentPrefix(api),
		}),
		credential,
		pipeline.MethodFactoryMarker(),
	}

	httpClient := &http.Client{Transport: r}
	// Create a pipeline that uses httpClient to make requests.
	p := pipeline.NewPipeline(f, pipeline.Options{
		HTTPSender: pipeline.FactoryFunc(func(next pipeline.Policy, po *pipeline.PolicyOptions) pipeline.PolicyFunc {
			return func(ctx context.Context, request pipeline.Request) (pipeline.Response, error) {
				r, err := httpClient.Do(request.WithContext(ctx))
				if err != nil {
					err = pipeline.NewError(err, "HTTP request failed")
				}
				return pipeline.NewHTTPResponse(r), err
			}
		}),
	})
	return p, done, httpClient
}

// NewAzureKeyVaultTestClient creates a *http.Client for Azure KeyVault test recordings.
func NewAzureKeyVaultTestClient(ctx context.Context, t *testing.T) (func(), *http.Client) {
	mode := recorder.ModeReplaying
	if *Record {
		mode = recorder.ModeRecording
	}

	azMatchers := &replay.ProviderMatcher{}
	r, done, err := replay.NewRecorder(t, mode, azMatchers, t.Name())
	if err != nil {
		t.Fatalf("unable to initialize recorder: %v", err)
	}

	return done, &http.Client{Transport: r}
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
