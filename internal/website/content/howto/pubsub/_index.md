---
title: "Pub/Sub"
date: 2019-03-26T09:44:06-07:00
showInSidenav: true
---

The [`pubsub` package][] provides an easy and portable way to interact with
publish/subscribe systems. This guide shows how to work with pubsub
in the Go CDK.

<!--more-->

The [publish/subscribe model][] allows parts of a system to publish messages
that other parts of a system may subscribe to. This is commonly used to
arrange for work to happen at some point after an interactive frontend
request is finished or in other event-driven computing.

The [`pubsub` package][] supports operations to publish messages to a topic and
to subscribe to receive messages from a topic.

Subpackages contain driver implementations of pubsub for various services,
including Cloud and on-prem solutions. You can develop your application
locally using [`mempubsub`][], then deploy it to multiple Cloud providers with
minimal initialization reconfiguration.

[publish/subscribe model]: https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern
[`pubsub` package]: https://godoc.org/gocloud.dev/pubsub
[`mempubsub`]: https://godoc.org/gocloud.dev/pubsub/mempubsub

