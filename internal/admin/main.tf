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

terraform {
  backend "gcs" {
    bucket = "go-cloud-tfstate"
  }

  required_version = "~>0.12"
}

provider "github" {
  version      = "~> 2.0"
  organization = "google"
  token        = var.github_token
}

provider "google" {
  version = "~> 2.5"
}

module "go_cloud_repo" {
  source       = "./repository"
  name         = "go-cloud"
  description  = "The Go Cloud Development Kit (Go CDK): A library and tools for open cloud development in Go."
  homepage_url = "https://gocloud.dev/"

  topics = [
    "cloud",
    "golang",
    "portable",
    "server",
    "multi-cloud",
    "hybrid-cloud",
    "go",
    "aws",
    "gcp",
    "azure",
  ]
}

module "wire_repo" {
  source      = "./repository"
  name        = "wire"
  description = "Compile-time Dependency Injection for Go"

  topics = [
    "go",
    "golang",
    "dependency-injection",
    "codegen",
    "initialization",
  ]
}

