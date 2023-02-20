// Copyright 2020 The Go Cloud Development Kit Authors
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

package gcpsecretmanager

import (
	"context"
	"errors"
	"fmt"
	"testing"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This constant records the project used for the last --record.
// If you want to use --record mode,
// 1. Update this constant to your GCP project ID.
// 2. Ensure that the "Secret Manager API" is enabled for your project.
// TODO(issue #300): Use Terraform to get this.
const projectID = "go-cloud-test-216917"

func secretKey(secretID string) string {
	return "projects/" + projectID + "/secrets/" + secretID
}

type harness struct {
	client *secretmanager.Client
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	ctx := context.Background()
	conn, done := setup.NewGCPgRPCConn(ctx, t, "secretmanager.googleapis.com:443", "runtimevar")

	client, err := secretmanager.NewClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	return &harness{
		client: client,
		closer: func() {
			_ = client.Close()
			done()
		},
	}, nil
}

func (h *harness) MakeWatcher(_ context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, secretKey(name), decoder, nil)
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	_, err := h.client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   "projects/" + projectID,
		SecretId: name,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
			Labels: map[string]string{
				"project": "runtimevar",
			},
		},
	})
	if err != nil {
		return err
	}

	// Add initial secret version.
	_, err = h.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  secretKey(name),
		Payload: &secretmanagerpb.SecretPayload{Data: val},
	})

	return err
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	_, err := h.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  secretKey(name),
		Payload: &secretmanagerpb.SecretPayload{Data: val},
	})

	return err
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	return h.client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{Name: secretKey(name)})
}

func (h *harness) Close() {
	h.closer()
}

func (h *harness) Mutable() bool { return true }

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness, []drivertest.AsTest{verifyAs{}})
}

type verifyAs struct{}

func (verifyAs) Name() string {
	return "verify As"
}

func (verifyAs) SnapshotCheck(s *runtimevar.Snapshot) error {
	var v *secretmanagerpb.AccessSecretVersionResponse
	if !s.As(&v) {
		return errors.New("Snapshot.As failed")
	}
	return nil
}

func (verifyAs) ErrorCheck(v *runtimevar.Variable, err error) error {
	var s *status.Status
	if !v.ErrorAs(err, &s) {
		return errors.New("runtimevar.ErrorAs failed")
	}
	return nil
}

// Secretmanager-specific tests.

func TestEquivalentError(t *testing.T) {
	tests := []struct {
		Err1, Err2 error
		Want       bool
	}{
		{Err1: errors.New("not grpc"), Err2: errors.New("not grpc"), Want: true},
		{Err1: errors.New("not grpc"), Err2: errors.New("not grpc but different")},
		{Err1: errors.New("not grpc"), Err2: status.Errorf(codes.Internal, "fail")},
		{Err1: status.Errorf(codes.Internal, "fail"), Err2: status.Errorf(codes.InvalidArgument, "fail")},
		{Err1: status.Errorf(codes.Internal, "fail"), Err2: status.Errorf(codes.Internal, "fail"), Want: true},
	}

	for _, test := range tests {
		got := equivalentError(test.Err1, test.Err2)
		if got != test.Want {
			t.Errorf("%v vs %v: got %v want %v", test.Err1, test.Err2, got, test.Want)
		}
	}
}

func TestNoConnectionError(t *testing.T) {
	ctx := context.Background()
	creds, err := setup.FakeGCPCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Connect to the Secret Manager service.
	client, cleanup, err := Dial(ctx, creds.TokenSource)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	key := SecretKey("gcp-project-id", "secret-name")
	v, err := OpenVariable(client, key, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}()

	_, err = v.Watch(ctx)
	if err == nil {
		t.Error("got nil want error")
	}
}

func TestOpenVariable(t *testing.T) {
	cleanup := setup.FakeGCPDefaultCredentials(t)
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret", false},
		// OK, hierarchical key name.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret2", false},
		// OK, setting decoder.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret?decoder=string", false},
		// Missing projects prefix.
		{"gcpsecretmanager://project/myproject/secrets/mysecret", true},
		// Missing project.
		{"gcpsecretmanager://projects//secrets/mysecret", true},
		// Missing configs.
		{"gcpsecretmanager://projects/myproject/mysecret", true},
		// Missing secretID with trailing slash.
		{"gcpsecretmanager://projects/myproject/secrets/", true},
		// Missing secretID.
		{"gcpsecretmanager://projects/myproject/secrets", true},
		// Invalid decoder.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret?decoder=notadecoder", true},
		// Invalid param.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret?param=value", true},
		// Setting wait.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret?wait=1m", false},
		// Invalid wait.
		{"gcpsecretmanager://projects/myproject/secrets/mysecret?wait=xx", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		if err := openVariable(ctx, test.URL); (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func openVariable(ctx context.Context, URL string) (err error) {
	var v *runtimevar.Variable
	v, err = runtimevar.OpenVariable(ctx, URL)
	defer func() {
		if v == nil {
			return
		}

		if closeErr := v.Close(); closeErr != nil {
			if grpcErr, ok := closeErr.(*gcerr.Error); ok && grpcErr.Code != gcerr.Canceled {
				err = fmt.Errorf("close failed: %v. prev error: %v", closeErr, err)
			}
		}
	}()

	return err
}
