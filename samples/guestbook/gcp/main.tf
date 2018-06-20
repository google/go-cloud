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

provider "google" {
  version = "~> 1.13"
  project = "${var.project}"
}

provider "random" {
  version = "~> 1.3"
}

# TODO(light): Add cloudbuild.googleapis.com resource.

# Service account for the running server

resource "google_service_account" "server" {
  account_id   = "${var.server_service_account_name}"
  display_name = "Guestbook Server"
}

resource "google_service_account_key" "server" {
  service_account_id = "${google_service_account.server.name}"
}

# Stackdriver Tracing

# TODO(light): Add cloudtrace.googleapis.com resource.

resource "google_project_iam_member" "server_trace" {
  role   = "roles/cloudtrace.agent"
  member = "serviceAccount:${google_service_account.server.email}"
}

# Cloud SQL

resource "google_project_service" "sql" {
  service            = "sql.googleapis.com"
  disable_on_destroy = false
}

# TODO(light): Add sqladmin.googleapis.com resource.

resource "google_sql_database_instance" "guestbook" {
  name             = "${var.db_instance}"
  database_version = "MYSQL_5_6"
  region           = "${var.region}"

  settings {
    tier      = "db-f1-micro"
    disk_size = 10            # GiB
  }

  depends_on = ["google_project_service.sql"]
}

resource "google_sql_database" "guestbook" {
  name     = "guestbook"
  instance = "${google_sql_database_instance.guestbook.name}"

  provisioner "local-exec" {
    # TODO(light): Reuse credentials from Terraform.
    command = "'${path.module}'/provision-db.sh '${google_sql_database_instance.guestbook.project}' '${google_service_account.db_access.email}' '${var.db_instance}' guestbook '${google_sql_user.root.password}' < '${path.module}'/../schema.sql"
  }
}

resource "random_string" "db_password" {
  keepers = {
    project = "${var.project}"
    db_name = "${var.db_instance}"
    region  = "${var.region}"
  }

  special = false
  length  = 20
}

resource "google_sql_user" "root" {
  name     = "root"
  instance = "${google_sql_database_instance.guestbook.name}"
  password = "${random_string.db_password.result}"
}

resource "google_sql_user" "guestbook" {
  name     = "guestbook"
  instance = "${google_sql_database_instance.guestbook.name}"
  host     = "cloudsqlproxy~%"
}

resource "google_service_account" "db_access" {
  account_id   = "${var.db_access_service_account_name}"
  display_name = "Guestbook Database Access"
}

resource "google_project_iam_member" "server_cloudsql" {
  role   = "roles/cloudsql.client"
  member = "serviceAccount:${google_service_account.server.email}"
}

resource "google_project_iam_member" "db_access_cloudsql" {
  role   = "roles/cloudsql.client"
  member = "serviceAccount:${google_service_account.db_access.email}"
}

# Runtime Configurator

resource "google_project_service" "runtimeconfig" {
  service            = "runtimeconfig.googleapis.com"
  disable_on_destroy = false
}

resource "google_runtimeconfig_config" "guestbook" {
  name = "guestbook"

  depends_on = ["google_project_service.runtimeconfig"]
}

resource "google_runtimeconfig_variable" "motd" {
  name   = "motd"
  parent = "${google_runtimeconfig_config.guestbook.name}"
  text   = "ohai from GCP runtime configuration"
}

resource "google_project_iam_member" "server_runtimeconfig" {
  role   = "roles/runtimeconfig.admin"
  member = "serviceAccount:${google_service_account.server.email}"
}

# Google Cloud Storage

resource "google_project_service" "storage" {
  service            = "storage.googleapis.com"
  disable_on_destroy = false
}

# TODO(light): Add storage-api.googleapis.com resource.

locals {
  bucket_name = "go-guestbook-${random_string.bucket_name.result}"
}

resource "random_string" "bucket_name" {
  keepers = {
    project = "${var.project}"
    region  = "${var.region}"
  }

  special = false
  length  = 32
}

resource "google_storage_bucket" "guestbook" {
  name          = "${local.bucket_name}"
  storage_class = "REGIONAL"
  location      = "${var.region}"

  # Set to avoid calling Compute API.
  # See https://github.com/hashicorp/terraform/issues/13109
  project = "${var.project}"

  depends_on = ["google_project_service.storage"]
}

resource "google_storage_bucket_iam_member" "guestbook_server_view" {
  bucket = "${google_storage_bucket.guestbook.name}"
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.server.email}"
}

# Kubernetes Engine

resource "google_project_service" "container" {
  service            = "container.googleapis.com"
  disable_on_destroy = false
}

resource "google_container_cluster" "guestbook" {
  name               = "${var.cluster_name}"
  initial_node_count = 3

  node_config {
    machine_type = "n1-standard-1"
    disk_size_gb = 50

    oauth_scopes = [
      "https://www.googleapis.com/auth/compute",
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]
  }

  # Needed for Kubernetes provider below.
  enable_legacy_abac = true

  depends_on = ["google_project_service.container"]
}

provider "kubernetes" {
  version = "~> 1.1"

  host = "https://${google_container_cluster.guestbook.endpoint}"

  client_certificate     = "${base64decode(google_container_cluster.guestbook.master_auth.0.client_certificate)}"
  client_key             = "${base64decode(google_container_cluster.guestbook.master_auth.0.client_key)}"
  cluster_ca_certificate = "${base64decode(google_container_cluster.guestbook.master_auth.0.cluster_ca_certificate)}"
}

resource "kubernetes_secret" "guestbook_creds" {
  metadata {
    name = "guestbook-key"
  }

  data {
    key.json = "${base64decode(google_service_account_key.server.private_key)}"
  }
}
