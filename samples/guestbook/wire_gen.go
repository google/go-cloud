// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package main

import (
	context "context"
	sql "database/sql"
	"log"
	http "net/http"
	"strings"

	client "github.com/aws/aws-sdk-go/aws/client"
	session "github.com/aws/aws-sdk-go/aws/session"
	mysql "github.com/go-sql-driver/mysql"
	blob "github.com/google/go-cloud/blob"
	fileblob "github.com/google/go-cloud/blob/fileblob"
	gcsblob "github.com/google/go-cloud/blob/gcsblob"
	s3blob "github.com/google/go-cloud/blob/s3blob"
	gcp "github.com/google/go-cloud/gcp"
	cloudmysql "github.com/google/go-cloud/mysql/cloudmysql"
	rdsmysql "github.com/google/go-cloud/mysql/rdsmysql"
	requestlog "github.com/google/go-cloud/requestlog"
	runtimevar "github.com/google/go-cloud/runtimevar"
	filevar "github.com/google/go-cloud/runtimevar/filevar"
	paramstore "github.com/google/go-cloud/runtimevar/paramstore"
	runtimeconfigurator "github.com/google/go-cloud/runtimevar/runtimeconfigurator"
	server "github.com/google/go-cloud/server"
	sdserver "github.com/google/go-cloud/server/sdserver"
	xrayserver "github.com/google/go-cloud/server/xrayserver"
	trace "go.opencensus.io/trace"
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
	session2, err := session.NewSessionWithOptions(options)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	xRay := xrayserver.NewXRayClient(session2)
	exporter, cleanup3, err := xrayserver.NewExporter(xRay)
	if err != nil {
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	sampler := trace.AlwaysSample()
	options2 := &server.Options{
		RequestLogger:         ncsaLogger,
		HealthChecks:          v,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: sampler,
	}
	server2 := server.New(options2)
	bucket, err := awsBucket(ctx, session2, flags)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	client2 := paramstore.NewClient(session2)
	variable, err := awsMOTDVar(ctx, client2, flags)
	if err != nil {
		cleanup3()
		cleanup2()
		cleanup()
		return nil, nil, err
	}
	application2 := newApplication(server2, db, bucket, variable)
	return application2, func() {
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
	server2 := server.New(options)
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
	application2 := newApplication(server2, db, bucket, variable)
	return application2, func() {
		cleanup3()
		cleanup2()
		cleanup()
	}, nil
}

func setupGAE(ctx context.Context, flags *cliFlags) (*application, func(), error) {
	stackdriverLogger := sdserver.NewRequestLogger()
	db, err := cloudmysql.OpenGAE(ctx)
	if err != nil {
		return nil, nil, err
	}
	v, cleanup := appHealthChecks(db)
	credentials, err := gcp.DefaultCredentials(ctx)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	projectID, err := gcp.DefaultProjectID(credentials)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)
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
	server2 := server.New(options)
	roundTripper := gcp.DefaultTransport()
	httpClient, err := gcp.NewHTTPClient(roundTripper, tokenSource)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
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
	application2 := newApplication(server2, db, bucket, variable)
	return application2, func() {
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
	server2 := server.New(options)
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
	application2 := newApplication(server2, db, bucket, variable)
	return application2, func() {
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
	b := flags.bucket
	if strings.TrimSpace(b) == "" {
		log.Fatal("gcpBucket: bucket name is empty")
	}
	return gcsblob.OpenBucket(ctx, flags.bucket, client2)
}

func gcpSQLParams(id gcp.ProjectID, flags *cliFlags) *cloudmysql.Params {
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
