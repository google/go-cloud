# TODO(rvangent): Add comments explaining.

locals {
  gcpkms_url = "gcpkms://${google_kms_crypto_key.key.self_link}"
}

resource "google_kms_key_ring" "keyring" {
  name     = local.gocdk_random_name
  location = "global"
}

resource "google_kms_crypto_key" "key" {
  name     = local.gocdk_random_name
  key_ring = "${google_kms_key_ring.keyring.self_link}"
}
