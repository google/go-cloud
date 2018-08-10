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

output "cluster_name" {
  value       = "${local.cluster_name}"
  description = "GKE cluster name."
}

output "cluster_zone" {
  value       = "${google_container_cluster.cluster.zone}"
  description = "GCP zone that the GKE cluster is in."
}
