# Contribute Bot Webhook Listener

Contribute Bot Webhook Listener is a microservice that listens for
[GitHub webhook][] events and publishes them to a [Cloud Pub/Sub][] topic.

[GitHub webhook]: https://developer.github.com/webhooks/
[Cloud Pub/Sub]: https://cloud.google.com/pubsub/docs/publisher

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
gcloud app deploy .
```
