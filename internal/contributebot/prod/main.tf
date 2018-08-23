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
  backend "gcs" {
    bucket = "go-cloud-contribute-bot-tfstate"
  }

  required_version = "~>0.11"
}

provider "google" {
  version = "~> 1.15"
  project = "go-cloud-contribute-bot"
}

variable "github_app_key" {
  default     = ""
  description = "PEM-encoded GitHub application private key. This defaults to empty for bootstrapping reasons."
}

module "contributebot" {
  source         = ".."
  project        = "go-cloud-contribute-bot"
  zone           = "us-central1-c"
  github_app_key = "${var.github_app_key}"
}
