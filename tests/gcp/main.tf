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

provider "random" {
  version = "~> 1.3"
}

resource "google_project_service" "cloudbuild" {
  service            = "cloudbuild.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "trace" {
  service            = "cloudtrace.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "container" {
  service            = "container.googleapis.com"
  disable_on_destroy = false
}

resource "random_id" "cluster" {
  keepers {
    project = "${var.project}"
    zone = "${var.zone}"
  }
  byte_length = 4
}

locals {
  cluster_name = "go-cloud-test-${random_id.cluster.hex}"
}

resource "google_container_cluster" "cluster" {
  name               = "${local.cluster_name}"
  zone               = "${var.zone}"
  initial_node_count = 1

  node_config {
    machine_type = "n1-standard-1"
    disk_size_gb = 10

    oauth_scopes = [
      "https://www.googleapis.com/auth/compute",
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/trace.append",
    ]
  }

  depends_on = ["google_project_service.container"]
}
