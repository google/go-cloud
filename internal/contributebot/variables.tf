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

terraform {
  backend "gcs" {}
}

variable "project" {
  type        = "string"
  description = "Project to set up."
}

variable "zone" {
  type        = "string"
  description = "GCP zone to create the GKE cluster in, like 'us-central1-a'. See https://cloud.google.com/compute/docs/regions-zones/ for valid values."
}

variable "worker_service_account_name" {
  default     = "contributebot"
  description = "The username part of the service account email that will be used for the worker running inside the GKE cluster."
}

variable "cluster_name" {
  default     = "contributebot-cluster"
  description = "The GKE cluster name."
}

variable "github_app_key" {
  default = ""
  description = "PEM-encoded GitHub application private key. This defaults to empty for bootstrapping reasons."
}
