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

package gcpfirestore

import (
	"context"
	"io"
	"net"
	"testing"

	"cloud.google.com/go/firestore"
	ts "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// A nativeCodec encodes and decodes structs using the cloud.google.com/go/firestore
// client. Since that package doesn't export its codec, we have to go behind the
// scenes and intercept traffic at the gRPC level. We use interceptors to do that. (A
// mock server would have worked too.)
type nativeCodec struct {
	client *firestore.Client
	doc    *pb.Document
}

func newNativeCodec() (*nativeCodec, error) {
	// Establish a gRPC server, just so we have a connection to hang the interceptors on.
	srv := grpc.NewServer()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		if err := srv.Serve(l); err != nil {
			panic(err) // we should never get an error because we just connect and stop
		}
	}()
	nc := &nativeCodec{}

	conn, err := grpc.Dial(l.Addr().String(),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithUnaryInterceptor(nc.interceptUnary),
		grpc.WithStreamInterceptor(nc.interceptStream))
	if err != nil {
		return nil, err
	}
	conn.Close()
	srv.Stop()
	nc.client, err = firestore.NewClient(context.Background(), "P", option.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}
	return nc, nil
}

// Intercept all unary (non-streaming) RPCs. The only one we should ever get is a Commit, for
// the Create call in Encode.
// If this completes successfully, the encoded *pb.Document will be in c.doc.
func (c *nativeCodec) interceptUnary(_ context.Context, method string, req, res interface{}, _ *grpc.ClientConn, _ grpc.UnaryInvoker, _ ...grpc.CallOption) error {
	c.doc = req.(*pb.CommitRequest).Writes[0].GetUpdate()
	res.(*pb.CommitResponse).WriteResults = []*pb.WriteResult{{}}
	return nil
}

// Intercept all streaming RPCs. The only one we should ever get is a BatchGet, for the Get
// call in Decode.
// Before this is called, c.doc must be set to the *pb.Document to be returned from the call.
func (c *nativeCodec) interceptStream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return &clientStream{ctx: ctx, doc: c.doc}, nil
}

// clientStream is a fake client stream. It returns a single document, then terminates.
type clientStream struct {
	ctx context.Context
	doc *pb.Document
}

func (cs *clientStream) RecvMsg(m interface{}) error {
	if cs.doc != nil {
		cs.doc.CreateTime = &ts.Timestamp{}
		cs.doc.UpdateTime = &ts.Timestamp{}
		m.(*pb.BatchGetDocumentsResponse).Result = &pb.BatchGetDocumentsResponse_Found{Found: cs.doc}
		cs.doc = nil
		return nil
	}
	return io.EOF
}

func (cs *clientStream) Context() context.Context     { return cs.ctx }
func (cs *clientStream) SendMsg(m interface{}) error  { return nil }
func (cs *clientStream) Header() (metadata.MD, error) { return nil, nil }
func (cs *clientStream) Trailer() metadata.MD         { return nil }
func (cs *clientStream) CloseSend() error             { return nil }

// Encode a Go value into a Firestore proto document.
func (c *nativeCodec) Encode(x interface{}) (*pb.Document, error) {
	_, err := c.client.Collection("C").Doc("D").Create(context.Background(), x)
	if err != nil {
		return nil, err
	}
	return c.doc, nil
}

// Decode value, which must be a *pb.Document, into dest.
func (c *nativeCodec) Decode(value *pb.Document, dest interface{}) error {
	c.doc = value
	docsnap, err := c.client.Collection("C").Doc("D").Get(context.Background())
	if err != nil {
		return err
	}
	return docsnap.DataTo(dest)
}

func TestNativeCodec(t *testing.T) {
	nc, err := newNativeCodec()
	if err != nil {
		t.Fatal(err)
	}
	type S struct {
		A int
	}
	want := S{3}
	fields, err := nc.Encode(&want)
	if err != nil {
		t.Fatal(err)
	}
	var got S
	if err := nc.Decode(fields, &got); err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
