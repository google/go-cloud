// Copyright 2018 The Go Cloud Development Kit Authors
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

package gcpruntimeconfig

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This constant records the project used for the last --record.
// If you want to use --record mode,
// 1. Update this constant to your GCP project name (not number!).
// 2. Ensure that the "Runtime Configuration API" is enabled for your project.
// TODO(issue #300): Use Terraform to get this.
const projectID = "go-cloud-test-216917"

const (
	// config is the runtimeconfig high-level config that variables sit under.
	config = "go_cloud_runtimeconfigurator_test"
)

func resourceName(name string) ResourceName {
	return ResourceName{
		ProjectID: projectID,
		Config:    config,
		Variable:  name,
	}
}

type harness struct {
	client pb.RuntimeConfigManagerClient
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	ctx := context.Background()
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "runtimevar")
	client := pb.NewRuntimeConfigManagerClient(conn)
	rn := resourceName("")
	// Ignore errors if the config already exists.
	_, _ = client.CreateConfig(ctx, &pb.CreateConfigRequest{
		Parent: "projects/" + rn.ProjectID,
		Config: &pb.RuntimeConfig{
			Name:        rn.configPath(),
			Description: t.Name(),
		},
	})
	return &harness{
		client: client,
		closer: func() {
			_, _ = client.DeleteConfig(ctx, &pb.DeleteConfigRequest{Name: rn.configPath()})
			done()
		},
	}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, resourceName(name), decoder, nil), nil
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	rn := resourceName(name)
	_, err := h.client.CreateVariable(ctx, &pb.CreateVariableRequest{
		Parent: rn.configPath(),
		Variable: &pb.Variable{
			Name:     rn.String(),
			Contents: &pb.Variable_Value{Value: val},
		},
	})
	return err
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	rn := resourceName(name)
	_, err := h.client.UpdateVariable(ctx, &pb.UpdateVariableRequest{
		Name: rn.String(),
		Variable: &pb.Variable{
			Contents: &pb.Variable_Value{Value: val},
		},
	})
	return err
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	rn := resourceName(name)
	_, err := h.client.DeleteVariable(ctx, &pb.DeleteVariableRequest{Name: rn.String()})
	return err
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
	var v *pb.Variable
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

// Runtimeconfigurator-specific tests.

func TestEquivalentError(t *testing.T) {
	tests := []struct {
		Err1, Err2 error
		Want       bool
	}{
		{Err1: errors.New("not grpc"), Err2: errors.New("not grpc"), Want: true},
		{Err1: errors.New("not grpc"), Err2: errors.New("not grpc but different")},
		{Err1: errors.New("not grpc"), Err2: grpc.Errorf(codes.Internal, "fail")},
		{Err1: grpc.Errorf(codes.Internal, "fail"), Err2: grpc.Errorf(codes.InvalidArgument, "fail")},
		{Err1: grpc.Errorf(codes.Internal, "fail"), Err2: grpc.Errorf(codes.Internal, "fail"), Want: true},
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

	// Connect to the Runtime Configurator service.
	client, cleanup, err := Dial(ctx, creds.TokenSource)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	name := ResourceName{
		ProjectID: "gcp-project-id",
		Config:    "cfg-name",
		Variable:  "cfg-variable-name",
	}
	v, err := OpenVariable(client, name, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.Watch(context.Background())
	if err == nil {
		t.Error("got nil want error")
	}
}

func TestResourceNameFromURL(t *testing.T) {
	tests := []struct {
		URL     string
		WantErr bool
		Want    ResourceName
	}{
		{"gcpruntimeconfig://proj1/cfg1/var1", false, ResourceName{"proj1", "cfg1", "var1"}},
		{"gcpruntimeconfig://proj2/cfg2/var2", false, ResourceName{"proj2", "cfg2", "var2"}},
		{"gcpruntimeconfig://proj/cfg/var/morevar", false, ResourceName{"proj", "cfg", "var/morevar"}},
		{"gcpruntimeconfig://proj/cfg", true, ResourceName{}},
		{"gcpruntimeconfig://proj/cfg/", true, ResourceName{}},
		{"gcpruntimeconfig:///cfg/var", true, ResourceName{}},
		{"gcpruntimeconfig://proj//var", true, ResourceName{}},
	}
	for _, test := range tests {
		u, err := url.Parse(test.URL)
		if err != nil {
			t.Fatal(err)
		}
		got, gotErr := newResourceNameFromURL(u)
		if (gotErr != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, gotErr, test.WantErr)
		}
		if got != test.Want {
			t.Errorf("%s: got %v want %v", test.URL, got, test.Want)
		}
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
		{"gcpruntimeconfig://myproject/mycfg/myvar", false},
		// OK, hierarchical key name.
		{"gcpruntimeconfig://myproject/mycfg/myvar1/myvar2", false},
		// OK, setting decoder.
		{"gcpruntimeconfig://myproject/mycfg/myvar?decoder=string", false},
		// Missing project ID.
		{"gcpruntimeconfig:///mycfg/myvar", true},
		// Empty config.
		{"gcpruntimeconfig://myproject//myvar", true},
		// Empty key name.
		{"gcpruntimeconfig://myproject/mycfg/", true},
		// Missing key name.
		{"gcpruntimeconfig://myproject/mycfg", true},
		// Invalid decoder.
		{"gcpruntimeconfig://myproject/mycfg/myvar?decoder=notadecoder", true},
		// Invalid param.
		{"gcpruntimeconfig://myproject/mycfg/myvar?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := runtimevar.OpenVariable(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
