// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package main

import (
	"context"
	"database/sql"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-sql-driver/mysql"
	"github.com/google/go-cloud/blob"
	"github.com/google/go-cloud/blob/fileblob"
	"github.com/google/go-cloud/blob/gcsblob"
	"github.com/google/go-cloud/blob/s3blob"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/mysql/cloudmysql"
	"github.com/google/go-cloud/mysql/rdsmysql"
	"github.com/google/go-cloud/requestlog"
	"github.com/google/go-cloud/runtimevar"
	"github.com/google/go-cloud/runtimevar/filevar"
	"github.com/google/go-cloud/runtimevar/paramstore"
	"github.com/google/go-cloud/runtimevar/runtimeconfigurator"
	"github.com/google/go-cloud/server"
	"github.com/google/go-cloud/server/sdserver"
	"github.com/google/go-cloud/server/xrayserver"
	"go.opencensus.io/trace"
	"log"
	"net/http"
	"os"
)

// Injectors from inject_aws.go:

func setupAWS(ctx context.Context, flags *cliFlags) (*application, func(), error) {
	ncsaLogger := xrayserver.NewRequestLogger()
	client := _wireClientValue
	certFetcher := &rdsmysql.CertFetcher{
		Client: client,
	}
	params := awsSQLParams(flags)
	db, cleanup, err := rdsmysql.Open(ctx, certFetcher, params)
	if err != nil {
		return nil, nil, err
	}
	v, cleanup2 := appHealthChecks(db)
	options := _wireOptionsValue
	sessionSession, err := session.NewSessionWithOptions(options)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	xRay := xrayserver.NewXRayClient(sessionSession)
	exporter, cleanup3, err := xrayserver.NewExporter(xRay)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	sampler := trace.AlwaysSample()
	serverOptions := &server.Options{
		RequestLogger:         ncsaLogger,
		HealthChecks:          v,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: sampler,
	}
	serverServer := server.New(serverOptions)
	bucket, err := awsBucket(ctx, sessionSession, flags)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	paramstoreClient := paramstore.NewClient(sessionSession)
	variable, err := awsMOTDVar(ctx, paramstoreClient, flags)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	mainApplication := newApplication(serverServer, db, bucket, variable)
	return mainApplication, func() {
		cleanup3()
		cleanup2()
		cleanup()
	}, nil
}

var (
	_wireClientValue  = http.DefaultClient
	_wireOptionsValue = session.Options{}
)

// Injectors from inject_gcp.go:

func setupGCP(ctx context.Context, flags *cliFlags) (*application, func(), error) {
	stackdriverLogger := sdserver.NewRequestLogger()
	roundTripper := gcp.DefaultTransport()
	credentials, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		return nil, nil, err
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)
	httpClient, err := gcp.NewHTTPClient(roundTripper, tokenSource)
	if err != nil {
		return nil, nil, err
	}
	remoteCertSource := cloudmysql.NewCertSource(httpClient)
	projectID, err := gcp.DefaultProjectID(credentials)
	if err != nil {
		return nil, nil, err
	}
	params := gcpSQLParams(projectID, flags)
	db, err := cloudmysql.Open(ctx, remoteCertSource, params)
	if err != nil {
		return nil, nil, err
	}
	v, cleanup := appHealthChecks(db)
	exporter, err := sdserver.NewExporter(projectID, tokenSource)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	sampler := trace.AlwaysSample()
	options := &server.Options{
		RequestLogger:         stackdriverLogger,
		HealthChecks:          v,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: sampler,
	}
	serverServer := server.New(options)
	bucket, err := gcpBucket(ctx, flags, httpClient)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	runtimeConfigManagerClient, cleanup2, err := runtimeconfigurator.Dial(ctx, tokenSource)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	client := runtimeconfigurator.NewClient(runtimeConfigManagerClient)
	variable, cleanup3, err := gcpMOTDVar(ctx, client, projectID, flags)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	mainApplication := newApplication(serverServer, db, bucket, variable)
	return mainApplication, func() {
		cleanup3()
		cleanup2()
		cleanup()
	}, nil
}

// Injectors from inject_local.go:

func setupLocal(ctx context.Context, flags *cliFlags) (*application, func(), error) {
	logger := _wireLoggerValue
	db, err := dialLocalSQL(flags)
	if err != nil {
		return nil, nil, err
	}
	v, cleanup := appHealthChecks(db)
	exporter := _wireExporterValue
	sampler := trace.AlwaysSample()
	options := &server.Options{
		RequestLogger:         logger,
		HealthChecks:          v,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: sampler,
	}
	serverServer := server.New(options)
	bucket, err := localBucket(flags)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	variable, cleanup2, err := localRuntimeVar(flags)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	mainApplication := newApplication(serverServer, db, bucket, variable)
	return mainApplication, func() {
		cleanup2()
		cleanup()
	}, nil
}

var (
	_wireLoggerValue   = requestlog.Logger(nil)
	_wireExporterValue = trace.Exporter(nil)
)

// inject_aws.go:

func awsBucket(ctx context.Context, cp client.ConfigProvider, flags *cliFlags) (*blob.Bucket, error) {
	return s3blob.OpenBucket(ctx, cp, flags.bucket)
}

func awsSQLParams(flags *cliFlags) *rdsmysql.Params {
	return &rdsmysql.Params{
		Endpoint: flags.dbHost,
		Database: flags.dbName,
		User:     flags.dbUser,
		Password: flags.dbPassword,
	}
}

func awsMOTDVar(ctx context.Context, client2 *paramstore.Client, flags *cliFlags) (*runtimevar.Variable, error) {
	return client2.NewVariable(flags.motdVar, runtimevar.StringDecoder, &paramstore.WatchOptions{
		WaitTime: flags.motdVarWaitTime,
	})
}

// inject_gcp.go:

func gcpBucket(ctx context.Context, flags *cliFlags, client2 *gcp.HTTPClient) (*blob.Bucket, error) {
	return gcsblob.OpenBucket(ctx, flags.bucket, client2)
}

func gcpSQLParams(id gcp.ProjectID, flags *cliFlags) *cloudmysql.Params {
	if os.Getenv("GAE_ENV") == "standard" {
		p, err := cloudmysql.ParamsFromEnv()
		if err != nil {
			log.Fatalf("Failed to get cloudsql params from env: %v", err)
		}
		return p
	}
	return &cloudmysql.Params{
		ProjectID: string(id),
		Region:    flags.cloudSQLRegion,
		Instance:  flags.dbHost,
		Database:  flags.dbName,
		User:      flags.dbUser,
		Password:  flags.dbPassword,
	}
}

func gcpMOTDVar(ctx context.Context, client2 *runtimeconfigurator.Client, project gcp.ProjectID, flags *cliFlags) (*runtimevar.Variable, func(), error) {
	name := runtimeconfigurator.ResourceName{
		ProjectID: string(project),
		Config:    flags.runtimeConfigName,
		Variable:  flags.motdVar,
	}
	v, err := client2.NewVariable(name, runtimevar.StringDecoder, &runtimeconfigurator.WatchOptions{
		WaitTime: flags.motdVarWaitTime,
	})
	if err != nil {
		return nil, nil, err
	}
	return v, func() { v.Close() }, nil
}

// inject_local.go:

func localBucket(flags *cliFlags) (*blob.Bucket, error) {
	return fileblob.NewBucket(flags.bucket)
}

func dialLocalSQL(flags *cliFlags) (*sql.DB, error) {
	cfg := &mysql.Config{
		Net:                  "tcp",
		Addr:                 flags.dbHost,
		DBName:               flags.dbName,
		User:                 flags.dbUser,
		Passwd:               flags.dbPassword,
		AllowNativePasswords: true,
	}
	return sql.Open("mysql", cfg.FormatDSN())
}

func localRuntimeVar(flags *cliFlags) (*runtimevar.Variable, func(), error) {
	v, err := filevar.NewVariable(flags.motdVar, runtimevar.StringDecoder, &filevar.WatchOptions{
		WaitTime: flags.motdVarWaitTime,
	})
	if err != nil {
		return nil, nil, err
	}
	return v, func() { v.Close() }, nil
}
