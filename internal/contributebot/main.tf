# Copyright 2018 Google LLC
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

# Stackdriver Tracing

resource "google_project_service" "trace" {
  service            = "cloudtrace.googleapis.com"
  project            = "${var.project}"
  disable_on_destroy = false
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
}

resource "google_pubsub_topic_iam_policy" "github_events" {
  topic       = "${google_pubsub_topic.github_events.name}"
  project     = "${var.project}"
  policy_data = "${data.google_iam_policy.github_events.policy_data}"
}
