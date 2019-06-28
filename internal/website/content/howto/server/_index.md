---
title: "Server"
date: 2019-06-21T10:36:43-07:00
draft: true
showInSidenav: true
---

The Go CDK's `server` package provides a pre-configured HTTP server with diagnostic hooks for request logging, health checks, and trace exporting via OpenCensus. These guides will show you how to start up and shut down the server, as well as how to work with the request logging, health checks, and trace exporting.

## Starting up the server

The GO CDK Server constructor takes an `http.Handler` and an `Options` struct. The simplest way to start the server is to use the `http.DefaultServeMux` and pass `nil` for the options.

{{< goexample src="gocloud.dev/server.ExampleServer_New" imports="0" >}}

The `Options` Struct 

### Adding a request logger

- default behavior
- how to specify something

### Adding health checks

- default behavior
- how to specify something

### Trace exporting with OpenCensus

- default behavior
- how to specify something

### Specifying the `http.Handler` (or router)

- default behavior
- how to specify something

### ListenAndServe

- call with an address

## Shutting down the server

- cleanup?