package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"go.opencensus.io/trace"
	"gocloud.dev/gcp"
	"gocloud.dev/health"
	"gocloud.dev/server"
	"gocloud.dev/server/sdserver"
)

type GlobalMonitoredResource struct {
	projectId string
}

func (g GlobalMonitoredResource) MonitoredResource() (string, map[string]string) {
	return "global", map[string]string{"project_id": g.projectId}
}

func helloHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Hello\n")
}

func mainHandler(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Welcome to the home page!\n")
}

type customHealthCheck struct {
	mu      sync.RWMutex
	healthy bool
}

func (h *customHealthCheck) CheckHealth() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.healthy {
		return errors.New("not ready yet!")
	}
	return nil
}

func main() {
	addr := flag.String("listen", ":8080", "HTTP port to listen on")
	doTrace := flag.Bool("trace", true, "Export traces to Stackdriver")
	flag.Parse()

	ctx := context.Background()
	credentials, err := gcp.DefaultCredentials(ctx)

	if err != nil {
		log.Fatal(err)
	}
	tokenSource := gcp.CredentialsTokenSource(credentials)
	projectId, err := gcp.DefaultProjectID(credentials)
	if err != nil {
		log.Fatal(err)
	}

	var exporter trace.Exporter
	if *doTrace {
		fmt.Println("Exporting traces to StackDriver")
		mr := GlobalMonitoredResource{projectId: string(projectId)}
		exporter, _, err = sdserver.NewExporter(projectId, tokenSource, mr)
		if err != nil {
			log.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/hello", helloHandler)
	mux.HandleFunc("/", mainHandler)

	myCheck := new(customHealthCheck)
	time.AfterFunc(10*time.Second, func() {
		myCheck.mu.Lock()
		defer myCheck.mu.Unlock()
		myCheck.healthy = true
	})

	options := &server.Options{
		RequestLogger:         sdserver.NewRequestLogger(),
		HealthChecks:          []health.Checker{myCheck},
		TraceExporter:         exporter,
		DefaultSamplingPolicy: trace.AlwaysSample(),
		Driver:                &server.DefaultDriver{},
	}

	s := server.New(mux, options)
	fmt.Printf("Listening on %s\n", *addr)

	err = s.ListenAndServe(*addr)
	if err != nil {
		log.Fatal(err)
	}
}
