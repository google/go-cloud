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
  account_id   = "contributebot"
  project      = "${var.project}"
  display_name = "Contribute Bot Server"
}

resource "google_service_account_key" "worker" {
  service_account_id = "${google_service_account.worker.name}"
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

# Kubernetes Engine

resource "google_project_service" "container" {
  service            = "container.googleapis.com"
  disable_on_destroy = false
}

resource "google_container_cluster" "contributebot" {
  name               = "contributebot-cluster"
  project            = "${var.project}"
  zone               = "${var.zone}"
  initial_node_count = 3

  node_config {
    machine_type = "n1-standard-1"
    disk_size_gb = 10

    oauth_scopes = [
      "https://www.googleapis.com/auth/compute",
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]
  }

  # Needed for Kubernetes provider below.
  enable_legacy_abac = true

  depends_on = ["google_project_service.container"]
}

provider "kubernetes" {
  version = "~> 1.1"

  host = "https://${google_container_cluster.contributebot.endpoint}"

  client_certificate     = "${base64decode(google_container_cluster.contributebot.master_auth.0.client_certificate)}"
  client_key             = "${base64decode(google_container_cluster.contributebot.master_auth.0.client_key)}"
  cluster_ca_certificate = "${base64decode(google_container_cluster.contributebot.master_auth.0.cluster_ca_certificate)}"
}

resource "kubernetes_secret" "worker_service_account" {
  metadata {
    name = "worker-service-account"
  }

  data {
    key.json = "${base64decode(google_service_account_key.worker.private_key)}"
  }
}

resource "kubernetes_secret" "github_app_key" {
  metadata {
    name = "github-app-key"
  }

  data {
    key.pem = "${var.github_app_key}"
  }
}
