# Copyright 2018 The Go Cloud Development Kit Authors
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

variable "project" {
  type        = string
  description = "Project to set up."
}

variable "zone" {
  type        = string
  description = "GCP zone to create the GKE cluster in, like 'us-central1-a'. See https://cloud.google.com/compute/docs/regions-zones/ for valid values."
}

variable "github_app_key" {
  default     = ""
  description = "PEM-encoded GitHub application private key. This defaults to empty for bootstrapping reasons."
}

