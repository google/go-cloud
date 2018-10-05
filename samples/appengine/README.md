# AppEngine Sample

This directory holds a simple "Hello world!" AppEngine app that uses
[server.Server](https://github.com/google/go-cloud/blob/master/server/server.go).

## Prerequisites

You will need to install the following software to run this sample:

-   [Go](https://golang.org/doc/install)
-   [gcloud CLI](https://cloud.google.com/sdk/downloads)

## Deploying

Run the following in this `samples/appengine` directory:

```shell
# Build the binary.
go build
# Deploy it to AppEngine.
gcloud app deploy
# Open a browser to the app.
gcloud app browse
```

Try browsing to the `/healthz/readiness` page that `server.Server` adds a
handler for.
