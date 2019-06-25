# TODO(rvangent): Add comments explaining.

locals {
  gcpruntimeconfig_url = "gcpruntimeconfig://projects/${data.google_project.project.project_id}/configs/${google_runtimeconfig_config.config.name}/variables/${google_runtimeconfig_variable.var.name}?decoder=string"
}

resource "google_project_service" "runtimeconfig" {
  service            = "runtimeconfig.googleapis.com"
  disable_on_destroy = false
}

resource "google_runtimeconfig_config" "config" {
  name = "${local.gocdk_random_name}"

  depends_on = ["google_project_service.runtimeconfig"]
}

resource "google_runtimeconfig_variable" "var" {
  name   = "${local.gocdk_random_name}"
  parent = "${google_runtimeconfig_config.config.name}"
  text   = "initial value of GCP Runtimeconfigurator config variable"
}
