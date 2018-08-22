// Copyright 2018 The Go Cloud Authors
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

package runtimeconfigurator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cloud/internal/testing/setup"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cloud/runtimevar/drivertest"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
)

// This constant records the project used for the last --record.
// If you want to use --record mode,
// 1. Update this constant to your GCP project name (not number!).
// 2. Ensure that the "Runtime Configuration API" is enabled for your project.
// TODO(issue #300): Use Terraform to get this.
const projectID = "google.com:rvangent-testing-prod"

const (
	// config is the runtimeconfig high-level config that variables sit under.
	config      = "go_cloud_runtimeconfigurator_test"
	description = "Config for test variables created by runtimeconfigurator_test.go"
)

func resourceName(name string) ResourceName {
	return ResourceName{
		ProjectID: projectID,
		Config:    config,
		Variable:  name,
	}
}

type harness struct {
	t      *testing.T
	client *Client
	closer func()
}

func newHarness(t *testing.T) drivertest.Harness {
	ctx := context.Background()
	client, done := newConfigClient(ctx, t)
	rn := resourceName("")
	// Ignore errors if the config already exists.
	_, _ = client.client.CreateConfig(ctx, &pb.CreateConfigRequest{
		Parent: "projects/" + rn.ProjectID,
		Config: &pb.RuntimeConfig{
			Name:        rn.configPath(),
			Description: t.Name(),
		},
	})
	return &harness{
		t:      t,
		client: client,
		closer: func() {
			_, _ = client.client.DeleteConfig(ctx, &pb.DeleteConfigRequest{Name: rn.configPath()})
			done()
		},
	}
}

func (h *harness) MakeVar(ctx context.Context, name string, decoder *runtimevar.Decoder) *runtimevar.Variable {
	rn := resourceName(name)
	v, err := h.client.NewVariable(ctx, rn, decoder, &WatchOptions{WaitTime: 5 * time.Millisecond})
	if err != nil {
		h.t.Fatal(err)
	}
	return v
}

func (h *harness) CreateVariable(ctx context.Context, name string, val []byte) {
	rn := resourceName(name)
	if _, err := h.client.client.CreateVariable(ctx, &pb.CreateVariableRequest{
		Parent: rn.configPath(),
		Variable: &pb.Variable{
			Name:     rn.String(),
			Contents: &pb.Variable_Value{Value: val},
		},
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) UpdateVariable(ctx context.Context, name string, val []byte) {
	rn := resourceName(name)
	if _, err := h.client.client.UpdateVariable(ctx, &pb.UpdateVariableRequest{
		Variable: &pb.Variable{
			Name:     rn.String(),
			Contents: &pb.Variable_Value{Value: val},
		},
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) DeleteVariable(ctx context.Context, name string) {
	rn := resourceName(name)
	if _, err := h.client.client.DeleteVariable(ctx, &pb.DeleteVariableRequest{Name: rn.String()}); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	drivertest.RunConformanceTests(t, newHarness)
}

// GCP-specific unit tests.
// TODO(rvangent): Delete most of these as they are moved into drivertest.

// Ensure that watcher implements driver.Watcher.
var _ driver.Watcher = &watcher{}

func TestInitialStringWatch(t *testing.T) {
	ctx := context.Background()

	client, done := newConfigClient(ctx, t)
	defer done()

	rn := ResourceName{
		ProjectID: projectID,
		Config:    config,
		desc:      description,
		Variable:  "TestStringWatch",
	}

	want := "facepalm: 🤦"
	_, done, err := createStringVariable(ctx, client.client, rn, want)
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	variable, err := client.NewVariable(ctx, rn, runtimevar.StringDecoder, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("got error %v; want nil", err)
	}
	if diff := cmp.Diff(got.Value, want); diff != "" {
		t.Errorf("got diff %v; want nil", diff)
	}
}

func TestInitialJSONWatch(t *testing.T) {
	ctx := context.Background()

	client, done := newConfigClient(ctx, t)
	defer done()

	rn := ResourceName{
		ProjectID: projectID,
		Config:    config,
		desc:      description,
		Variable:  "TestJSONWatch",
	}

	type home struct {
		Person string `json:"Person"`
		Home   string `json:"Home"`
	}
	var jsonDataPtr *home
	want := &home{"Batman", "Gotham"}
	_, done, err := createByteVariable(ctx, client.client, rn, []byte(`{"Person": "Batman", "Home": "Gotham"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	variable, err := client.NewVariable(ctx, rn, runtimevar.NewDecoder(jsonDataPtr, runtimevar.JSONDecode), nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("got error %v; want nil", err)
	}
	if diff := cmp.Diff(got.Value.(*home), want); diff != "" {
		t.Errorf("got diff %v; want nil", diff)
	}
}

func TestContextCanceledBeforeFirstWatch(t *testing.T) {
	ctx := context.Background()

	client, done := newConfigClient(ctx, t)
	defer done()

	rn := ResourceName{
		ProjectID: projectID,
		Config:    config,
		desc:      description,
		Variable:  "TestWatchCancel",
	}

	variable, err := client.NewVariable(ctx, rn, runtimevar.StringDecoder, nil)
	if err != nil {
		t.Fatalf("Client.NewConfig returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = variable.Watch(ctx)
	if err == nil {
		t.Fatal("Variable.Watch returned nil error, expecting an error from canceling")
	}
}

func TestContextCanceledInbetweenWatchCalls(t *testing.T) {
	ctx := context.Background()

	client, done := newConfigClient(ctx, t)
	defer done()

	rn := ResourceName{
		ProjectID: projectID,
		Config:    config,
		desc:      description,
		Variable:  "TestWatchInbetweenCancel",
	}

	_, done, err := createStringVariable(ctx, client.client, rn, "getting canceled")
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	variable, err := client.NewVariable(ctx, rn, runtimevar.StringDecoder, nil)
	if err != nil {
		t.Fatalf("Client.NewConfig returned error: %v", err)
	}

	_, err = variable.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = variable.Watch(ctx)
	if err == nil {
		t.Fatal("Variable.Watch returned nil error, expecting an error from canceling")
	}
}

func TestWatchObservesChange(t *testing.T) {
	ctx := context.Background()

	client, done := newConfigClient(ctx, t)
	defer done()

	rn := ResourceName{
		ProjectID: projectID,
		Config:    config,
		desc:      description,
		Variable:  "TestWatchObserveChange",
	}

	want := "cash 💰 change"
	_, done, err := createStringVariable(ctx, client.client, rn, want)
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	variable, err := client.NewVariable(ctx, rn, runtimevar.StringDecoder, &WatchOptions{WaitTime: 1 * time.Second})
	if err != nil {
		t.Fatalf("Client.NewConfig returned error: %v", err)
	}
	got, err := variable.Watch(ctx)
	switch {
	case err != nil:
		t.Fatal(err)
	case got.Value != want:
		t.Errorf("got %v; want %v", got.Value, want)
	}

	// Update the value and see that watch sees the new value.
	want = "be the change you want to see in the 🌎"
	_, err = updateVariable(ctx, client.client, rn, want)
	if err != nil {
		t.Fatal(err)
	}

	got, err = variable.Watch(ctx)
	switch {
	case err != nil:
		t.Fatal(err)
	case got.Value != want:
		t.Errorf("got %v; want %v", got.Value, want)
	}
}

func newConfigClient(ctx context.Context, t *testing.T) (*Client, func()) {
	conn, done := setup.NewGCPgRPCConn(ctx, t, endPoint)
	return NewClient(pb.NewRuntimeConfigManagerClient(conn)), done
}

// createConfig creates a fresh config. It will always overwrite any previous configuration,
// thus it is not thread safe.
func createConfig(ctx context.Context, client pb.RuntimeConfigManagerClient, rn ResourceName) (*pb.RuntimeConfig, error) {
	// No need to handle this error; either the config doesn't exist (good) or the test
	// will fail on the create step and requires human intervention anyway.
	_ = deleteConfig(ctx, client, rn)
	return client.CreateConfig(ctx, &pb.CreateConfigRequest{
		Parent: "projects/" + rn.ProjectID,
		Config: &pb.RuntimeConfig{
			Name:        rn.configPath(),
			Description: rn.desc,
		},
	})
}

func deleteConfig(ctx context.Context, client pb.RuntimeConfigManagerClient, rn ResourceName) error {
	_, err := client.DeleteConfig(ctx, &pb.DeleteConfigRequest{
		Name: rn.configPath(),
	})

	return err
}

func createByteVariable(ctx context.Context, client pb.RuntimeConfigManagerClient, rn ResourceName, value []byte) (*pb.Variable, func(), error) {
	if _, err := createConfig(ctx, client, rn); err != nil {
		return nil, nil, fmt.Errorf("unable to create parent config for %+v: %v", rn, err)
	}

	v, err := client.CreateVariable(ctx, &pb.CreateVariableRequest{
		Parent: rn.configPath(),
		Variable: &pb.Variable{
			Name:     rn.String(),
			Contents: &pb.Variable_Value{Value: value},
		},
	})

	return v, func() { _ = deleteConfig(ctx, client, rn) }, err
}

func createStringVariable(ctx context.Context, client pb.RuntimeConfigManagerClient, rn ResourceName, str string) (*pb.Variable, func(), error) {
	if _, err := createConfig(ctx, client, rn); err != nil {
		return nil, nil, fmt.Errorf("unable to create parent config for %+v: %v", rn, err)
	}

	v, err := client.CreateVariable(ctx, &pb.CreateVariableRequest{
		Parent: rn.configPath(),
		Variable: &pb.Variable{
			Name:     rn.String(),
			Contents: &pb.Variable_Text{Text: str},
		},
	})

	return v, func() { _ = deleteConfig(ctx, client, rn) }, err
}

func updateVariable(ctx context.Context, client pb.RuntimeConfigManagerClient, rn ResourceName, str string) (*pb.Variable, error) {
	return client.UpdateVariable(ctx, &pb.UpdateVariableRequest{
		Name: rn.String(),
		Variable: &pb.Variable{
			Name:     rn.String(),
			Contents: &pb.Variable_Text{Text: str},
		},
	})
}
