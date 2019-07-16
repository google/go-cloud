# This file creates a GCP Runtime Configurator variable for use with the
# "runtimevar/gcpruntimeconfig" driver.

locals {
  # The Go CDK URL for the variable: https://gocloud.dev/howto/runtimevar/#gcprc.
  gcpruntimeconfig_url = "gcpruntimeconfig://projects/${data.google_project.project.project_id}/configs/${google_runtimeconfig_config.config.name}/variables/${google_runtimeconfig_variable.var.name}?decoder=string"
}

# Enable a required API to use Runtime Configurator.
resource "google_project_service" "runtimeconfig" {
  service            = "runtimeconfig.googleapis.com"
  disable_on_destroy = false
}

# The configuration to hold our variable.
resource "google_runtimeconfig_config" "config" {
  name = local.gocdk_random_name

  depends_on = [google_project_service.runtimeconfig]
}

# The variable itself, with an initial value.
resource "google_runtimeconfig_variable" "var" {
  name   = local.gocdk_random_name
  parent = google_runtimeconfig_config.config.name
  text   = "initial value of GCP Runtimeconfigurator config variable"
}

