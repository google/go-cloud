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

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsv2config "github.com/aws/aws-sdk-go-v2/config"
	awsv2creds "github.com/aws/aws-sdk-go-v2/credentials"

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

func awsV2Config(ctx context.Context, region string, client *http.Client) (awsv2.Config, error) {
	// Provide fake creds if running in replay mode.
	var creds awsv2.CredentialsProvider
	if !*Record {
		creds = awsv2creds.NewStaticCredentialsProvider("FAKE_KEY", "FAKE_SECRET", "FAKE_SESSION")
	}
	return awsv2config.LoadDefaultConfig(
		ctx,
		awsv2config.WithHTTPClient(client),
		awsv2config.WithRegion(region),
		awsv2config.WithCredentialsProvider(creds),
		awsv2config.WithRetryer(func() awsv2.Retryer { return awsv2.NopRetryer{} }),
	)
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

// NewAWSv2Config creates a new aws.Config for testing against AWS.
// If the test is in --record mode, the test will call out to AWS, and the
// results are recorded in a replay file.
// Otherwise, the session reads a replay file and runs the test as a replay,
// which never makes an outgoing HTTP call and uses fake credentials.
// An initState is returned for tests that need a state to have deterministic
// results, for example, a seed to generate random sequences.
func NewAWSv2Config(ctx context.Context, t *testing.T, region string) (cfg awsv2.Config, rt http.RoundTripper, cleanup func(), initState int64) {
	client, cleanup, state := NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		r.RemoveQueryParams("X-Amz-Credential", "X-Amz-Signature", "X-Amz-Security-Token")
		r.RemoveRequestHeaders("Authorization", "Duration", "X-Amz-Security-Token")
		r.ClearHeaders("Amz-Sdk-Invocation-Id")
		r.ClearHeaders("X-Amz-Date")
		r.ClearQueryParams("X-Amz-Date")
		r.ClearHeaders("User-Agent") // AWS includes the Go version
		// The MessageAttributes parameter is a map, and so the values are
		// in randomized order, so we can't match against them. Just scrub
		// them and rely on the ordering.
		r.ScrubBody("MessageAttributes.*")
	})
	cfg, err := awsV2Config(ctx, region, client)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, client.Transport, cleanup, state
}

// NewGCPClient creates a new HTTPClient for testing against GCP.
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
		r.ClearHeaders("X-Goog-Gcs-Idempotency-Token")
		r.ClearHeaders("User-Agent")
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

// NewAzureTestBlobClient creates a new connection for testing against Azure Blob.
func NewAzureTestBlobClient(ctx context.Context, t *testing.T) (*http.Client, func()) {
	client, cleanup, _ := NewRecordReplayClient(ctx, t, func(r *httpreplay.Recorder) {
		r.RemoveQueryParams("se", "sig", "st")
		r.RemoveQueryParams("X-Ms-Date")
		r.ClearQueryParams("blockid")
		r.ClearHeaders("X-Ms-Date")
		r.ClearHeaders("X-Ms-Version")
		r.ClearHeaders("User-Agent") // includes the full Go version
		// Yes, it's true, Azure does not appear to be internally
		// consistent about casing for BLock(l|L)ist.
		r.ScrubBody("<Block(l|L)ist><Latest>.*</Latest></Block(l|L)ist>")
	})
	return client, cleanup
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
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
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
// 1) Not on Github Actions.
// 2) On Github's Linux environment, where Docker is available.
func HasDockerTestEnvironment() bool {
	s := os.Getenv("RUNNER_OS")
	return s == "" || s == "Linux"
}
