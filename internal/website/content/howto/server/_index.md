---
title: "Server"
date: 2019-06-21T10:36:43-07:00
draft: true
showInSidenav: true
---

The Go CDK's `server` package provides a pre-configured HTTP server with 
diagnostic hooks for request logging, health checks, and trace exporting via 
OpenCensus. This guide will show you how to start up and shut down the server,
as well as how to work with the request logging and health checks.

The Go CDK includes a server package to provide some reasonable defaults, such
as timeouts, or the Apache standard format logger, and also to share best
practices like health checks.

## Starting up the server

The GO CDK Server constructor takes an `http.Handler` and an `Options` struct. 
The simplest way to start the server is to use the `http.DefaultServeMux` and
pass `nil` for the options.

{{< goexample src="gocloud.dev/server.ExampleServer_New" >}}

### Adding a request logger

You can use the `server.Options` struct to specify a request logger.

The example is shown with the Go CDK [`requestlog`](https://godoc.org/gocloud.dev/requestlog) package's `NCSALogger`.
To get logs in the Stackdriver JSON format, use `NewStackdriverLogger` in place
of `NewNCSALogger`.

{{< goexample src="gocloud.dev/server.ExampleServer_RequestLogger" >}}

### Adding health checks

The Go CDK `server` package affords a hook for you to define health checks for
your application and see the results at `/healthz/readiness`. Health checks are
an imortant part of application monitoring.

Because each application may have a different definition of what it means to be
"healthy", you will need to define a concrete type to implement the `health.Checker`
interface and define a `CheckHealth` method specific to your application.
```go
// customHealthCheck is an example health check. It implements the
// health.Checker interface and reports the server is healthy when the healthy
// field is set to true.
type customHealthCheck struct {
	mu      sync.RWMutex
	healthy bool
}

// customHealthCheck implements the health.Checker interface because it has a
// CheckHealth method.
func (h *customHealthCheck) CheckHealth() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.healthy {
		return errors.New("not ready yet!")
	}
	return nil
}
```

{{< goexample src="gocloud.dev/server.ExampleServer_HealthChecks" >}}
