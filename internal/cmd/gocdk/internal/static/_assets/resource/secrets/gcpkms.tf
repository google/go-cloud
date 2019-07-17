# This file creates GCP KMS keyring and key for use with the "secrets/gcpkms" driver.

locals {
  # The Go CDK URL for the key: https://gocloud.dev/howto/secrets/#gcp.
  gcpkms_url = "gcpkms://${google_kms_crypto_key.key.self_link}"
}

# The keyring to hold the key.
resource "google_kms_key_ring" "keyring" {
  name     = local.gocdk_random_name
  location = "global"
}

# The key itself.
resource "google_kms_crypto_key" "key" {
  name     = local.gocdk_random_name
  key_ring = "${google_kms_key_ring.keyring.self_link}"
}
