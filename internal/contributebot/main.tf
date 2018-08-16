# Copyright 2018 The Go Cloud Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

provider "google" {
  version = "~> 1.15"
  project = "${var.project}"
}

locals {
  appengine_service_account = "${var.project}@appspot.gserviceaccount.com"
}

resource "google_project_service" "cloudbuild" {
  service            = "cloudbuild.googleapis.com"
  project            = "${var.project}"
  disable_on_destroy = false
}

# Service account for the event worker

resource "google_service_account" "worker" {
  account_id   = "${var.worker_service_account_name}"
  project      = "${var.project}"
  display_name = "Contribute Bot Server"
}

# Stackdriver Tracing

resource "google_project_service" "trace" {
  service            = "cloudtrace.googleapis.com"
  project            = "${var.project}"
  disable_on_destroy = false
}

resource "google_project_iam_member" "worker_trace" {
  role    = "roles/cloudtrace.agent"
  project = "${var.project}"
  member  = "serviceAccount:${google_service_account.worker.email}"
}

# Pub/Sub

resource "google_pubsub_topic" "github_events" {
  name    = "contributebot-github-events"
  project = "${var.project}"
}

data "google_iam_policy" "github_events" {
  binding {
    role = "roles/pubsub.publisher"

    members = [
      "serviceAccount:${local.appengine_service_account}",
    ]
  }

  binding {
    role = "roles/pubsub.subscriber"

    members = [
      "serviceAccount:${google_service_account.worker.email}",
    ]
  }
}

resource "google_pubsub_topic_iam_policy" "github_events" {
  topic       = "${google_pubsub_topic.github_events.name}"
  project     = "${var.project}"
  policy_data = "${data.google_iam_policy.github_events.policy_data}"
}

resource "google_pubsub_subscription" "worker" {
  name    = "contributebot-github-events"
  topic   = "${google_pubsub_topic.github_events.id}"
  project = "${var.project}"
}

data "google_iam_policy" "worker_subscription" {
  binding {
    role = "roles/pubsub.subscriber"

    members = [
      "serviceAccount:${google_service_account.worker.email}",
    ]
  }

  binding {
    role = "roles/pubsub.viewer"

    members = [
      "serviceAccount:${google_service_account.worker.email}",
    ]
  }
}

resource "google_pubsub_subscription_iam_policy" "worker" {
  subscription = "${google_pubsub_subscription.worker.id}"
  project      = "${var.project}"
  policy_data  = "${data.google_iam_policy.worker_subscription.policy_data}"
}
