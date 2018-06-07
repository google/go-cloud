# Copyright 2018 Google LLC
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
  value = "${data.google_client_config.current.project}"
}

output "server_service_account" {
  value = "${google_service_account.server.email}"
}

output "db_access_service_account" {
  value = "${google_service_account.db_access.email}"
}

output "cluster_name" {
  value = "${var.cluster_name}"
}

output "cluster_zone" {
  value = "${google_container_cluster.guestbook.zone}"
}

output "bucket" {
  value = "${google_storage_bucket.guestbook.name}"
}

output "database_instance" {
  value = "${google_sql_database_instance.guestbook.name}"
}

output "database_root_password" {
  value = "${random_string.db_password.result}"
  sensitive = true
}

output "database_region" {
  value = "${local.db_region}"
}

output "motd_var_config" {
  value = "${google_runtimeconfig_config.guestbook.name}"
}

output "motd_var_name" {
  value = "${google_runtimeconfig_variable.motd.name}"
}
