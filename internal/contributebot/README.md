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

## Setup

To set up your own instance of Contribute Bot for local testing or deployment:

1.  [Create a new GCP project][].
1.  Set your project using `gcloud config set project PROJECTID`, where
    `PROJECTID` is the project's ID.
1.  Enable App Engine with `gcloud app create`.
1.  Create a bucket to store the [Terraform state][] using
    `gsutil mb -c regional -l us-west2 gs://some-bucket`, replacing
    `some-bucket` with a new unique name.
1.  Run `terraform init -backend-config 'bucket=some-bucket'`, replacing
    `some-bucket` with the same name you used in the last step,
1.  Create a [variable file][] for the settings from `variables.tf`. Skip
    `github_app_key` for now.
1.  Run `terraform apply` to set up the infrastructure.
1.  [Deploy the webhook][], creating a random webhook secret.
1.  [Create the GitHub application][], setting the webhook URL to
    `https://PROJECTID.appspot.com/webhook`, where `PROJECTID` is your GCP
    project ID. Make sure to give Read &amp; Write access to Issues, Pull
    Requests, Checks, and Read-only access to Repository metadata. Subscribe to
    pull request and issue events.
1.  Download a GitHub application secret key and copy the contents into your
    Terraform variable file for the `github_app_key` variable. (It's useful to
    use a ["here doc"][].)
1.  Run `terraform apply` again to update the secret material.

[Create a new GCP project]: https://console.cloud.google.com/projectcreate
[Create the GitHub application]: https://github.com/settings/apps/new
[Deploy the webhook]: webhook/README.md
["here doc"]: https://www.terraform.io/docs/configuration/syntax.html
[Terraform state]: https://www.terraform.io/docs/state/remote.html
[variable file]: https://www.terraform.io/docs/configuration/variables.html#variable-files

## Developing

To run Contribute Bot locally for testing:

1.  Create a GitHub repository for testing.
1.  Install the GitHub application on your test respository.
1.  Download a GitHub application secret key for your test application.
1.  Run `contributebot`, setting the flags for your test GCP project and GitHub
    application.
