---
title: "Server"
date: 2019-06-21T10:36:43-07:00
draft: true
showInSidenav: true
---

The Go CDK's `server` package provides a pre-configured HTTP server with diagnostic hooks for request logging, health checks, and trace exporting via OpenCensus. These guides will show you how to start up and shut down the server, as well as how to work with the request logging, health checks, and trace exporting.

The Go CDK includes a server package because??

## Starting up the server

The GO CDK Server constructor takes an `http.Handler` and an `Options` struct. The simplest way to start the server is to use the `http.DefaultServeMux` and pass `nil` for the options.

{{< goexample src="gocloud.dev/server.ExampleServer_New" >}}

### Adding a request logger

You can use the `server.Options` struct to specify a request logger.

The example is shown with the Go CDK [`requestlog`](https://godoc.org/gocloud.dev/requestlog) package's `NCSALogger`. To get logs in the Stackdriver JSON format, use `NewStackdriverLogger` in place of `NewNCSALogger`.

{{< goexample src="gocloud.dev/server.ExampleServer_RequestLogger" >}}

### Adding health checks

- default behavior
- how to specify something

{{< goexample src="gocloud.dev/server.ExampleServer_HealthChecks" >}}


### Trace exporting with OpenCensus

- default behavior
- how to specify something


## Shutting down the server

Like all Go CDK [portable types](https://gocloud.dev/concepts/structure/#portable-types-and-drivers), `server.Server`  has a `driver` field with an interface type (in this case, `driver.Server`). By default, the `server` package uses `http.Server` to satisfy its [driver interface](https://godoc.org/gocloud.dev/server/driver), but you can use other implementations if you prefer. Calling `Shutdown` on the server calls the driver's `Shutdown` method.

{{< goexample src="gocloud.dev/server.ExampleServer_Shutdown" >}}