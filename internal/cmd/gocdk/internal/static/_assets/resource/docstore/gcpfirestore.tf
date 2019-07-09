# TODO(rvangent): Add comments explaining.

locals {
  gcpfirestore_url = "firestore://projects/${data.google_project.project.project_id}/databases/(default)/documents/${local.gocdk_random_name}?name_field=Key"
}
