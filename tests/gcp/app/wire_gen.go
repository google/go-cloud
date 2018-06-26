// Code generated by gowire. DO NOT EDIT.

//go:generate gowire
//+build !wireinject

package main

import (
	context "context"
	gcp "github.com/google/go-x-cloud/gcp"
	health "github.com/google/go-x-cloud/health"
	server "github.com/google/go-x-cloud/server"
	sdserver "github.com/google/go-x-cloud/server/sdserver"
	trace "go.opencensus.io/trace"
)

// Injectors from inject.go:

func initialize(ctx context.Context) (*server.Server, error) {
	stackdriverLogger := sdserver.NewRequestLogger()
	v := []health.Checker{connection}
	credentials, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, err
	}
	projectID, err := gcp.DefaultProjectID(credentials)
	if err != nil {
		return nil, err
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)
	exporter, err := sdserver.NewExporter(projectID, tokenSource)
	if err != nil {
		return nil, err
	}
	sampler := trace.AlwaysSample()
	options := &server.Options{
		RequestLogger:         stackdriverLogger,
		HealthChecks:          v,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: sampler,
	}
	server2 := server.New(options)
	return server2, nil
}
