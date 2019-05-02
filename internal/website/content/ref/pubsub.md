---
title: "Pubsub"
date: 2019-02-21T16:21:29-08:00
aliases:
- /pages/pubsub/
---

Pub/Sub refers to implementations of the [publish-subscribe
pattern](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern),
wherein clients connect to a cloud service to subscribe to topics or publish
messages that could be delivered to subscribers.

Package `pubsub` provides an easy and portable way to interact with
publish/subscribe systems that have [at-least-once
delivery](https://en.wikipedia.org/wiki/Advanced_Message_Queuing_Protocol#Overview).

<!--more-->

[Top-level package documentation](https://godoc.org/gocloud.dev/pubsub)<br>
[How-to guides]({{< ref "/howto/pubsub/_index.md" >}})

## Supported Providers

* [Google Cloud Pub/Sub](https://godoc.org/gocloud.dev/pubsub/gcppubsub)
* [Amazon SNS+SQS](https://godoc.org/gocloud.dev/pubsub/awssnssqs)
* [Azure Service Bus](https://godoc.org/gocloud.dev/pubsub/azuresb)
* [RabbitMQ](https://godoc.org/gocloud.dev/pubsub/rabbitpubsub)
* [Kafka](https://godoc.org/gocloud.dev/pubsub/kafkapubsub)
* [NATS](https://godoc.org/gocloud.dev/pubsub/natspubsub)
* [In-memory local Pub/Sub](https://godoc.org/gocloud.dev/pubsub/mempubsub) -
  mainly useful for local testing

## Usage Samples

* [CLI Sample](https://github.com/google/go-cloud/tree/master/samples/gocdk-pubsub)
* [pubsub package examples](https://godoc.org/gocloud.dev/pubsub#pkg-examples)
