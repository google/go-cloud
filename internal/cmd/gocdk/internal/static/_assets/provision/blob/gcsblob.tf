# TODO(rvangent): Add comments explaining.

locals {
  gcsblob_bucket_url  = "gs://${google_storage_bucket.bucket.id}"
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
  name     = "gocdk-${random_id.gcsblob_bucket_name_suffix.hex}"
  location = "${local.gcp_storage_location}"

  depends_on = [
    "google_project_service.storage",
    "google_project_service.storage_api",
  ]
}
