# TODO(rvangent): Add comments explaining.

locals {
  gcsblob_bucket_name = "gocdk-${random_id.gcsblob_bucket_name_suffix.hex}"
  gcsblob_bucket_url  = "gs://${local.gcsblob_bucket_name}"
}

resource "random_id" "gcsblob_bucket_name_suffix" {
  byte_length = 16
}

resource "google_project_service" "storage" {
  service            = "storage-component.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "storage_api" {
  service            = "storage-api.googleapis.com"
  disable_on_destroy = false
}

resource "google_storage_bucket" "bucket" {
  name     = "${local.gcsblob_bucket_name}"
  location = "${local.gcs_bucket_location}"

  depends_on = [
    "google_project_service.storage",
    "google_project_service.storage_api",
  ]
}
