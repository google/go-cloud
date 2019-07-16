# A provider for Google Cloud Platform (GCP).
# https://www.terraform.io/docs/providers/google/index.html
provider "google" {
  version = "~> 2.0"
  project = local.gcp_project
}

# A datasource that returns info about the current GCP project.
# https://www.terraform.io/docs/providers/google/d/google_project.html
data "google_project" "project" {
}

