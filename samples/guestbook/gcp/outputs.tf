# Copyright 2018 The Go Cloud Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "project" {
  value       = "${var.project}"
  description = "The GCP project ID."
}

output "server_service_account" {
  value       = "${google_service_account.server.email}"
  description = "The service account email that will be used for the server running inside the GKE cluster."
}

output "db_access_service_account" {
  value       = "${google_service_account.db_access.email}"
  description = "The service account email that was used for provisioning the database."
}

output "cluster_name" {
  value       = "${var.cluster_name}"
  description = "GKE cluster name."
}

output "cluster_zone" {
  value       = "${google_container_cluster.guestbook.zone}"
  description = "GCP zone that the GKE cluster is in."
}

output "bucket" {
  value       = "${local.bucket_name}"
  description = "Name of the GCS bucket created to store images."
}

output "database_instance" {
  value       = "${google_sql_database_instance.guestbook.name}"
  description = "Cloud SQL instance name."
}

output "database_root_password" {
  value       = "${random_string.db_password.result}"
  sensitive   = true
  description = "The Cloud SQL instance password for root."
}

output "database_region" {
  value       = "${var.region}"
  description = "The Cloud SQL instance region."
}

output "motd_var_config" {
  value       = "${google_runtimeconfig_config.guestbook.name}"
  description = "The name of the Runtime Configurator config resource that contains the Message of the Day variable."
}

output "motd_var_name" {
  value       = "${google_runtimeconfig_variable.motd.name}"
  description = "The name of the Runtime Configurator variable inside the config resource that contains the Message of the Day."
}
