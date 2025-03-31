---
title: "Server"
date: 2019-06-21T10:36:43-07:00
showInSidenav: true
toc: true
---

The Go CDK's `server` package provides a pre-configured HTTP server with 
diagnostic hooks for request logging, health checks, and trace exporting via 
OpenCensus. This guide will show you how to start up and shut down the server,
as well as how to work with the request logging and health checks.

## Starting up the server

The Go CDK Server constructor takes an `http.Handler` and an `Options` struct. 
The simplest way to start the server is to use `http.DefaultServeMux` and
pass `nil` for the options.

{{< goexample src="gocloud.dev/server.ExampleServer" >}}

### Adding a request logger

You can use the `server.Options` struct to specify a request logger.

The example is shown with the Go CDK [`requestlog`](https://godoc.org/gocloud.dev/server/requestlog) package's `NCSALogger`.
To get logs in the Stackdriver JSON format, use `NewStackdriverLogger` in place
of `NewNCSALogger`.

{{< goexample src="gocloud.dev/server.ExampleServer_withRequestLogger" >}}

### Adding health checks

The Go CDK `server` package affords a hook for you to define health checks for
your application and see the results at `/healthz/readiness`. The server also
runs an endpoint at `/healthz/liveness`, which is a conventional name for a
liveness check and is where Kubernetes, if you are using it, will look.

Health checks are an important part of application monitoring, and readiness
checks are subtly different than liveness checks. The liveness check will return
`200 OK` if the server can serve requests. But because each application may have
a different definition of what it means to be "healthy" (perhaps your
application has a dependency on a back end service), you will need to define a
concrete type to implement the `health.Checker` interface and define a 
`CheckHealth` method specific to your application for readiness checks.

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

{{< goexample src="gocloud.dev/server.ExampleServer_withHealthChecks" >}}

## Other Usage Samples

* [Minimal server sample](https://github.com/google/go-cloud/tree/master/samples/server)
* [Guestbook sample](https://gocloud.dev/tutorials/guestbook/)
