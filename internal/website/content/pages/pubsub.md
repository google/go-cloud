---
title: "Pubsub"
---

Pubsub refers to implementations of the [publish-subscribe
pattern](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern),
wherein clients connect to a cloud service to subsribe to topics or publish
messages that could be delivered to subscribers.

Package `pubsub` provides an easy and portable way to interact with
publish/subscribe systems.

Top-level package documentation: https://godoc.org/gocloud.dev/pubsub

Supported providers:

* [GCP](https://godoc.org/gocloud.dev/pubsub/gcppubsub)
* [AWS](https://github.com/google/go-cloud/tree/master/pubsub/awspubsub)
* [Azure](https://github.com/google/go-cloud/tree/master/pubsub/azurepubsub)
* [In-memory local pubsub](https://godoc.org/gocloud.dev/pubsub/mempubsub) -
  mainly useful for local testing
* [RabbitMQ](https://godoc.org/gocloud.dev/pubsub/rabbitpubsub) - local
  implementation using the [RabbitMQ](https://www.rabbitmq.com/) message broker

Usage samples:

* [gcmsg sample](https://github.com/google/go-cloud/tree/master/samples/gcmsg)
* [pubsub package examples](https://godoc.org/gocloud.dev/pubsub#pkg-examples)
