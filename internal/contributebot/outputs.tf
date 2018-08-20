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

output "project" {
  value       = "${var.project}"
  description = "The GCP project ID."
}

output "github_events_topic" {
  value       = "projects/${var.project}/topics/${google_pubsub_topic.github_events.name}"
  description = "The fully qualified name of the topic."
}

output "github_events_worker_subscription" {
  value       = "${google_pubsub_subscription.worker.path}"
  description = "The fully qualified name of the event worker's subscription."
}

output "worker_service_account" {
  value       = "${google_service_account.worker.email}"
  description = "The service account email that will be used for the worker running inside the GKE cluster."
}

output "cluster_name" {
  value       = "${google_container_cluster.contributebot.name}"
  description = "GKE cluster name."
}

output "cluster_zone" {
  value       = "${var.zone}"
  description = "GCP zone that the GKE cluster is in."
}
