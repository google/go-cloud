# Go Cloud

[![](https://godoc.org/github.com/google/go-cloud?status.svg)](http://godoc.org/github.com/google/go-cloud)

This repo is a design experiment for a library making it possible to write Go programs across multiple cloud platforms.

```
$ go get -u github.com/google/go-cloud
```

You specify which cloud platforms you want to support when you build your application:

```
$ go build -tags 'gcp'         # build with GCP support
$ go build -tags 'gcp aws'     # build with GCP and AWS support
```

See [example/README.md](example/README.md) for a walkthrough of an example application.

## Resource URIs

Local and cloud resources are specified at runtime. A program will panic if it attempts to access resources on a cloud platform without having been built with support for that cloud platform. Support for local resources are always compiled into binaries using the cloud library.

### Logging

| Platform     | Service             | Resource URI                                   |
|--------------|---------------------|------------------------------------------------|
| Local        | OS file descriptors | `local://log_prefix:path/to/file.log`          |
| GCP          | [Stackdriver](https://cloud.google.com/logging/)         | `stackdriver://log_prefix:stackdriver_logName` |

### Configuration

| Platform     | Service                      | Resource URI                                   |
|--------------|------------------------------|------------------------------------------------|
| Local        | JSON file                    | `local://path/to/config_local.json`                  |
| GCP          | [Runtime Configurator](https://cloud.google.com/deployment-manager/runtime-configurator/)         | `grc://configName` |
| AWS          | JSON file in S3              | `s3://bucketName/objectName.json`|

### Storage

| Platform     | Service             | Resource URI                                   |
|--------------|------------------------------|------------------------------------------------|
| Local        | File                    | `local://path/to/file`                  |
| GCP          | [Cloud Storage](https://cloud.google.com/storage/)         | `gcs://bucketName/objectName` |
| AWS          | [S3](https://aws.amazon.com/s3/) | `s3://bucketName/objectName` |
