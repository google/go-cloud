# Contribute Bot

Contribute Bot is a small service (written using Go Cloud!) that performs
automated checks on issues and pull requests to help keep contributions
organized and easy to triage for maintainers.

Contribute Bot has two servers: a webhook endpoint and an event listener. The
webhook endpoint publishes events to a Cloud Pub/Sub topic that are eventually
processed by the event listener. GitHub has a
[10 second webhook response time limit][github-async] combined with a
[5000 request/hour API rate limit][github-ratelimit], so this adds buffering
with the assumption that incoming events are bursty.

[github-async]: https://developer.github.com/v3/guides/best-practices-for-integrators/#favor-asynchronous-work-over-synchronous
[github-ratelimit]: https://developer.github.com/v3/#rate-limiting

## Setup

To set up your own instance of Contribute Bot for local testing or deployment:

1.  [Create a new GCP project][].
1.  Set your project using `gcloud config set project PROJECTID`, where
    `PROJECTID` is the project's ID.
1.  Download default application credentials with `gcloud auth
    application-default login`.
1.  Enable App Engine with `gcloud app create`.
1.  Copy the `prod` directory to a directory called `dev`.
1.  In `dev/main.tf`, remove the `backend "gcs"` block and change the project
    IDs to your new GCP project.
1.  Run `terraform init` from the new `dev` directory.
1.  Run `terraform apply` to set up the infrastructure.
1.  [Deploy the webhook][], creating a random webhook secret.
1.  [Create the GitHub application][], setting the webhook URL to
    `https://PROJECTID.appspot.com/webhook`, where `PROJECTID` is your GCP
    project ID.
    *   Set the `Webhook secret` to the random webhook secret you created above.
    *   Make sure to give Read &amp; Write access to Issues, Pull Requests,
        Checks, and Read-only access to Repository metadata.
    *   Subscribe to pull request and issue events.
1.  Download a GitHub application secret key and copy the contents into a new
    Terraform [variable file][] in the `dev` directory, setting the
    `github_app_key` variable. It's useful to use a ["here doc"][]. Then run
    `terraform apply` again to update the secret material. Your variable file
    should look something like this:

```bash
contributebot/dev$ cat terraform.tfvars
github_app_key = <<EOF
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----
EOF
```

[Create a new GCP project]: https://console.cloud.google.com/projectcreate
[Create the GitHub application]: https://github.com/settings/apps/new
[Deploy the webhook]: webhook/README.md
["here doc"]: https://www.terraform.io/docs/configuration/syntax.html
[variable file]: https://www.terraform.io/docs/configuration/variables.html#variable-files

## Developing

To run Contribute Bot locally for testing:

1.  Create a GitHub repository for testing.
1.  Install the GitHub application on your test repository (`Settings >
    Developer Settings > Github Apps`, then `Edit` your app and select `Install
    App`).
1.  Download a GitHub application secret key for your test application.
1.  Run `contributebot`, setting the flags for your test GCP project and GitHub
    application. You can find the App ID under `About` on the Github page for
    your app. Example:

```go
go run . --project=your-project-name --github_app=42 --github_key=/foo.pem
```

## Deploying

### To production

To deploy an updated Contribute Bot to production, follow these steps.

```shell
# Build Docker image.
gcloud builds submit --config cloudbuild.yaml ../.. --project=go-cloud-contribute-bot

# Edit prod/k8s/contributebot.yaml and replace the image with the one
# you just created.

# Apply to cluster. Replace project and zone with the actual values.
gcloud container clusters get-credentials \
    --project=go-cloud-contribute-bot \
    --zone=us-central1-c \
    contributebot-cluster
kubectl apply -f prod/k8s

# Send a PR with the updated .yaml file.
```

To check that the deploy was successful, consult the contributebot-worker [Deployment Details][]

[Deployment Details]: https://pantheon.corp.google.com/kubernetes/deployment/us-central1-c/contributebot-cluster/default/contributebot-worker?project=go-cloud-contribute-bot&tab=history&deployment_overview_active_revisions_tablesize=50&duration=PT1H&pod_summary_list_tablesize=20&deployment_revision_history_tablesize=50


### Somewhere else

If you want to deploy to your own cluster, modify `k8s/contributebot.yaml` to
replace `go-cloud-contribute-bot` with your own project ID, and `15206` with
your own Github App ID. Run the commands above, using your own project ID
in the command line arguments instead of `go-cloud-contribute-bot`.

