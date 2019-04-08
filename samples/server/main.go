package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"go.opencensus.io/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/server"
	"gocloud.dev/server/sdserver"
)

type GlobalMonitoredResource struct {
	projectId string
}

func (g GlobalMonitoredResource) MonitoredResource() (string, map[string]string) {
	return "global", map[string]string{"project_id": g.projectId}
}

func main() {
	addr := flag.String("listen", ":8080", "HTTP port to listen on")

	ctx := context.Background()
	credentials, err := gcp.DefaultCredentials(ctx)

	if err != nil {
		log.Fatal(err)
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)

	projectId, err := gcp.DefaultProjectID(credentials)
	fmt.Printf("projectId = %s\n", projectId)
	if err != nil {
		log.Fatal(err)
	}
	mr := GlobalMonitoredResource{projectId: string(projectId)}
	exporter, _, err := sdserver.NewExporter(projectId, tokenSource, mr)

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Hello\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Welcome to the home page!\n")
	})

	options := &server.Options{
		RequestLogger:         sdserver.NewRequestLogger(),
		HealthChecks:          nil,
		TraceExporter:         exporter,
		DefaultSamplingPolicy: trace.AlwaysSample(),
		Driver:                &server.DefaultDriver{},
	}

	s := server.New(options)
	fmt.Printf("Listening on %s\n", *addr)
	s.ListenAndServe(*addr, mux)
}
