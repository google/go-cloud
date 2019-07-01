# TODO(rvangent): Add comments explaining.

locals {
  gcppubsub_topic_url        = "gcppubsub://projects/${google_pubsub_topic.topic.project}/topics/${google_pubsub_topic.topic.name}"
  gcppubsub_subscription_url = "gcppubsub://projects/${google_pubsub_subscription.subscription.project}/subscriptions/${google_pubsub_subscription.subscription.name}"
}

resource "google_project_service" "pubsub" {
  service            = "pubsub.googleapis.com"
  disable_on_destroy = false
}

resource "google_pubsub_topic" "topic" {
  name = "${local.gocdk_random_name}"

  depends_on = ["google_project_service.pubsub"]
}

resource "google_pubsub_subscription" "subscription" {
  name  = "${local.gocdk_random_name}"
  topic = "${google_pubsub_topic.topic.name}"
}
