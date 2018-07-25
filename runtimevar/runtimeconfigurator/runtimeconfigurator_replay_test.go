// +build replay

package runtimeconfigurator

import (
	"context"

	"github.com/dnaeon/go-vcr/recorder"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/testing/replay"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

func newConfigClient(ctx context.Context, logf func(string, ...interface{}), filepath string) (*Client, func(), error) {
	creds, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, err
	}

	mode := recorder.ModeReplaying
	rOpts, done, err := replay.NewGCPDialOptions(logf, mode, filepath, scrubber)
	if err != nil {
		return nil, nil, err
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
		grpc.WithPerRPCCredentials(oauth.TokenSource{gcp.CredentialsTokenSource(creds)}),
	}
	opts = append(opts, rOpts...)
	conn, err := grpc.DialContext(ctx, endPoint, opts...)
	if err != nil {
		return nil, nil, err
	}

	return NewClient(pb.NewRuntimeConfigManagerClient(conn)), done, nil
}
