provider "google" {
  version = "~> 2.0"
  project = "${local.gcp_project}"
}

data "google_project" "project" {}
