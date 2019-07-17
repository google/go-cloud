# This file creates a GCP topic and a subscription subscribed to it, for use
# with the "pubsub/gcppubsub" driver.

locals {
  # The Go CDK URL for the topic: https://gocloud.dev/howto/pubsub/publish#gcp.
  gcppubsub_topic_url = "gcppubsub://projects/${google_pubsub_topic.topic.project}/topics/${google_pubsub_topic.topic.name}"
  # The Go CDK URL for the subscription: https://gocloud.dev/howto/pubsub/subscribe#gcp.
  gcppubsub_subscription_url = "gcppubsub://projects/${google_pubsub_subscription.subscription.project}/subscriptions/${google_pubsub_subscription.subscription.name}"
}

resource "google_project_service" "pubsub" {
  service            = "pubsub.googleapis.com"
  disable_on_destroy = false
}

resource "google_pubsub_topic" "topic" {
  name = local.gocdk_random_name

  depends_on = [google_project_service.pubsub]
}

resource "google_pubsub_subscription" "subscription" {
  name  = local.gocdk_random_name
  topic = google_pubsub_topic.topic.name
}

