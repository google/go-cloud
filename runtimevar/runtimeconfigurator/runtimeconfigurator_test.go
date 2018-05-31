// Copyright 2018 Google LLC
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
	"codename/gcp"
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/rpcreplay"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/driver"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

// Ensure that watcher implements driver.Watcher.
var _ driver.Watcher = &watcher{}

// fakeServer partially implements runtimevarManagerServer for Client to connect to.  Prefill
// responses field with the ordered list of responses to GetVariable calls.
type fakeServer struct {
	pb.RuntimeConfigManagerServer
	responses []response
	index     int
}

type response struct {
	vrbl *pb.Variable
	err  error
}

func (s *fakeServer) GetVariable(context.Context, *pb.GetVariableRequest) (*pb.Variable, error) {
	if len(s.responses) == 0 {
		return nil, fmt.Errorf("fakeClient missing responses")
	}
	resp := s.responses[s.index]
	// Adjust index to next response for next call till it gets to last one, then keep using the
	// last one.
	if s.index < len(s.responses)-1 {
		s.index++
	}
	return resp.vrbl, resp.err
}

func setUp(t *testing.T, fs *fakeServer) (*Client, func()) {
	t.Helper()
	// Set up gRPC server.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("tcp listen failed: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterRuntimeConfigManagerServer(s, fs)
	// Run gRPC server on a background goroutine.
	go s.Serve(lis)

	// Set up client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	client := &Client{
		conn:   conn,
		client: pb.NewRuntimeConfigManagerClient(conn),
	}
	return client, func() {
		client.Close()
		s.Stop()
	}
}

type jsonData struct {
	Hello string `json:"hello"`
}

var (
	// Set wait timeout used for tests.
	watchOpt = &WatchOptions{
		WaitTime: 100 * time.Millisecond,
	}
	resourceName = ResourceName{
		ProjectID: "ID42",
		Config:    "config",
		Variable:  "greetings",
	}
	startTime = time.Now().Unix()
	jsonVar1  = &pb.Variable{
		Name:       "greetings",
		Contents:   &pb.Variable_Text{Text: `{"hello": "hello"}`},
		UpdateTime: &tspb.Timestamp{Seconds: startTime},
	}
	jsonVar2 = &pb.Variable{
		Name:       "greetings",
		Contents:   &pb.Variable_Value{Value: []byte(`{"hello": "hola"}`)},
		UpdateTime: &tspb.Timestamp{Seconds: startTime + 100},
	}
	jsonDataPtr *jsonData
)

func TestWatchReplay(t *testing.T) {
	ctx := context.Background()
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := rpcreplay.NewRecorder("watcher.replay", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{gcp.CredentialsTokenSource(creds)}),
	}
	opts = append(opts, rec.DialOptions()...)
	_, err = grpc.DialContext(ctx, endPoint, opts...)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWatch(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: jsonVar1},
			{vrbl: jsonVar2},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	variable, err := client.NewVariable(ctx, resourceName, runtimevar.NewDecoder(jsonDataPtr, runtimevar.JSONDecode), watchOpt)
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	got1, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}
	if diff := cmp.Diff(got1.Value.(*jsonData), &jsonData{"hello"}); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}

	got2, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}
	if diff := cmp.Diff(got2.Value.(*jsonData), &jsonData{"hola"}); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}
}

func TestCustomDecode(t *testing.T) {
	value := "hello world"
	strVar := &pb.Variable{
		Name:       "greetings",
		Contents:   &pb.Variable_Value{Value: []byte(value)},
		UpdateTime: &tspb.Timestamp{Seconds: startTime},
	}

	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: strVar},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	watchOpt := &WatchOptions{
		WaitTime: 500 * time.Millisecond,
	}
	variable, err := client.NewVariable(ctx, resourceName, runtimevar.NewDecoder("", stringDecode), watchOpt)
	if err != nil {
		t.Fatalf("Client.NewConfig returned error: %v", err)
	}

	got, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}
	if diff := cmp.Diff(got.Value.(string), value); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}
}

func stringDecode(b []byte, obj interface{}) error {
	// obj is a pointer to a string.
	v := reflect.ValueOf(obj).Elem()
	v.SetString(string(b))
	return nil
}

func TestWatchCancelledBeforeFirstWatch(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: jsonVar1},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	variable, err := client.NewVariable(ctx, resourceName, runtimevar.NewDecoder(jsonDataPtr, runtimevar.JSONDecode), watchOpt)
	if err != nil {
		t.Fatalf("Client.NewConfig returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = variable.Watch(ctx)
	if err == nil {
		t.Fatal("Variable.Watch returned nil error, expecting an error from cancelling")
	}
}

func TestContextCancelledInBetweenWatchCalls(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: jsonVar1},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	variable, err := client.NewVariable(ctx, resourceName, runtimevar.NewDecoder(jsonDataPtr, runtimevar.JSONDecode), watchOpt)
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
		t.Fatal("Variable.Watch returned nil error, expecting an error from cancelling")
	}
}

func TestWatchDeletedAndReset(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: jsonVar1},
			{err: status.Error(codes.NotFound, "deleted")},
			{vrbl: jsonVar2},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	variable, err := client.NewVariable(ctx, resourceName, runtimevar.NewDecoder(jsonDataPtr, runtimevar.JSONDecode), watchOpt)
	if err != nil {
		t.Fatalf("Client.NewConfig() returned error: %v", err)
	}

	prev, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}

	// Expect deleted error.
	if _, err := variable.Watch(ctx); err == nil {
		t.Fatalf("Variable.Watch returned nil, want error")
	}

	// Calling Watch again will poll for jsonVar2.
	got, err := variable.Watch(ctx)
	if err != nil {
		t.Fatalf("Variable.Watch returned error: %v", err)
	}
	if diff := cmp.Diff(got.Value.(*jsonData), &jsonData{"hola"}); diff != "" {
		t.Errorf("Snapshot.Value: %s", diff)
	}
	if !got.UpdateTime.After(prev.UpdateTime) {
		t.Errorf("Snapshot.UpdateTime is less than or equal to previous value")
	}
}
