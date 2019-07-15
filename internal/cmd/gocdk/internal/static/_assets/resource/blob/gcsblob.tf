# This file creates a GCS bucket for use with the "blob/gcsblob" driver.

locals {
  # The Go CDK URL for the bucket: https://gocloud.dev/howto/blob/#gcs.
  gcsblob_bucket_url = "gs://${google_storage_bucket.bucket.id}"
}

# Enable a couple of required APIs to use GCS.
resource "google_project_service" "storage" {
  service            = "storage-component.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "storage_api" {
  service            = "storage-api.googleapis.com"
  disable_on_destroy = false
}

# The actual bucket.
resource "google_storage_bucket" "bucket" {
  name          = local.gocdk_random_name
  location      = local.gcp_storage_location
  force_destroy = true

  depends_on = [
    google_project_service.storage,
    google_project_service.storage_api,
  ]
}

