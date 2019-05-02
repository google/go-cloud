---
title: "Pub/Sub"
date: 2019-03-26T09:44:06-07:00
showInSidenav: true
---

The [publish/subscribe model][] allows parts of a system to publish messages
that other parts of a system may subscribe to. This is commonly used to
arrange for work to happen at some point after an interactive frontend
request is finished or in other event-driven computing.

The Go CDK provides portable APIs for publishing messages and subscribing to
messages.

[publish/subscribe model]: https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern

<!--more-->
