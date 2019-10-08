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
	"fmt"
	"testing"

	"gocloud.dev/internal/testing/setup"
	"gocloud.dev/runtimevar"
	"gocloud.dev/runtimevar/driver"
	"gocloud.dev/runtimevar/drivertest"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// This constant records the project used for the last --record.
// If you want to use --record mode,
// 1. Update this constant to your GCP project ID.
// 2. Ensure that the "Runtime Configuration API" is enabled for your project.
// TODO(issue #300): Use Terraform to get this.
const projectID = "go-cloud-test-216917"

const (
	// configID is the runtimeconfig high-level config that variables sit under.
	configID = "go_cloud_runtimeconfigurator_test"
)

func configPath() string {
	return fmt.Sprintf("projects/%s/configs/%s", projectID, configID)
}

func variableKey(variableName string) string {
	return VariableKey(projectID, configID, variableName)
}

type harness struct {
	client pb.RuntimeConfigManagerClient
	closer func()
}

func newHarness(t *testing.T) (drivertest.Harness, error) {
	ctx := context.Background()
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint, "runtimevar")
	client := pb.NewRuntimeConfigManagerClient(conn)
	// Ignore errors if the config already exists.
	_, _ = client.CreateConfig(ctx, &pb.CreateConfigRequest{
		Parent: "projects/" + projectID,
		Config: &pb.RuntimeConfig{
			Name:        configPath(),
			Description: t.Name(),
		},
	})
	return &harness{
		client: client,
		closer: func() {
			_, _ = client.DeleteConfig(ctx, &pb.DeleteConfigRequest{Name: configPath()})
			done()
		},
	}, nil
}

func (h *harness) MakeWatcher(ctx context.Context, name string, decoder *runtimevar.Decoder) (driver.Watcher, error) {
	return newWatcher(h.client, variableKey(name), decoder, nil)
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) error {
	_, err := h.client.CreateVariable(ctx, &pb.CreateVariableRequest{
		Parent: configPath(),
		Variable: &pb.Variable{
			Name:     variableKey(name),
			Contents: &pb.Variable_Value{Value: val},
		},
	})
	return err
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) error {
	_, err := h.client.UpdateVariable(ctx, &pb.UpdateVariableRequest{
		Name: variableKey(name),
		Variable: &pb.Variable{
			Contents: &pb.Variable_Value{Value: val},
		},
	})
	return err
}

func (h *harness) DeleteVariable(ctx context.Context, name string) error {
	_, err := h.client.DeleteVariable(ctx, &pb.DeleteVariableRequest{Name: variableKey(name)})
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

	// Connect to the Runtime Configurator service.
	client, cleanup, err := Dial(ctx, creds.TokenSource)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	variableKey := VariableKey("gcp-project-id", "cfg-name", "cfg-variable-name")
	v, err := OpenVariable(client, variableKey, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	_, err = v.Watch(context.Background())
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
		{"gcpruntimeconfig://projects/myproject/configs/mycfg/variables/myvar", false},
		// OK, hierarchical key name.
		{"gcpruntimeconfig://projects/myproject/configs/mycfg/variables/myvar1/myvar2", false},
		// OK, setting decoder.
		{"gcpruntimeconfig://projects/myproject/configs/mycfg/variables/myvar?decoder=string", false},
		// Missing projects prefix.
		{"gcpruntimeconfig://project/myproject/configs/mycfg/variables/myvar", true},
		// Missing project.
		{"gcpruntimeconfig://projects//configs/mycfg/variables/myvar", true},
		// Missing configs.
		{"gcpruntimeconfig://projects/myproject/mycfg/variables/myvar", true},
		// Missing configID.
		{"gcpruntimeconfig://projects/myproject/configs//variables/myvar", true},
		// Missing variables.
		{"gcpruntimeconfig://projects/myproject/configs/mycfg//myvar", true},
		// Missing variable name.
		{"gcpruntimeconfig://projects/myproject/configs/mycfg/variables/", true},
		// Invalid decoder.
		{"gcpruntimeconfig://myproject/mycfg/myvar?decoder=notadecoder", true},
		// Invalid param.
		{"gcpruntimeconfig://myproject/mycfg/myvar?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		v, err := runtimevar.OpenVariable(ctx, test.URL)
		if v != nil {
			defer v.Close()
		}
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
