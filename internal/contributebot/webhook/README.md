# Contribute Bot Webhook Listener

Contribute Bot Webhook Listener is a microservice that listens for
[GitHub webhook][] events and publishes them to a [Cloud Pub/Sub][] topic.

You will need the [Go App Engine SDK][] to develop this server.

[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/publisher
[GitHub webhook]: https://developer.github.com/webhooks/
[Go App Engine SDK]: https://cloud.google.com/appengine/docs/standard/go/download

## Running Locally

```shell
cp app.yaml.in app.yaml
# Edit app.yaml to fill in configuration.
dev_appserver.py .
```


## Deploying

```shell
cp app.yaml.in app.yaml
# Edit app.yaml to fill in configuration.
# You may have to run `go get`.
# If you're using go 1.11+, you may have to set `GO111MODULE=off`.
gcloud app deploy .
```
