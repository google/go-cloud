# Contribute Bot

Contribute Bot is a small service (written using Go Cloud!) that performs
automated checks on issues and pull requests to help keep contributions
organized and easy to triage for maintainers.

Contribute Bot has two servers: a webhook endpoint and an event listener. The
webhook endpoint publishes events to a Cloud Pub/Sub topic that are eventually
processed by the event listener. GitHub has a [10 second webhook response time
limit][github-async] combined with a [5000 request/hour API rate
limit][github-ratelimit], so this adds buffering with the assumption that
incoming events are bursty.

[github-async]: https://developer.github.com/v3/guides/best-practices-for-integrators/#favor-asynchronous-work-over-synchronous
[github-ratelimit]: https://developer.github.com/v3/#rate-limiting
