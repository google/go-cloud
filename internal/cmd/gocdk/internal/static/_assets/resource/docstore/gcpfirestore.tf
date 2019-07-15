# This file creates an GCP Firestore collection for use with the
# "docstore/gcpfirestore" driver.

locals {
  # The Go CDK URL for the bucket: https://gocloud.dev/howto/docstore/#firestore.
  gcpfirestore_url = "firestore://projects/${data.google_project.project.project_id}/databases/(default)/documents/${local.gocdk_random_name}?name_field=Key"
}

# Databases are created automatically, so there's no need to provision one
# explicitly.
