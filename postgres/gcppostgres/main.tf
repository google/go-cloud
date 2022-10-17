# Copyright 2018 The Go Cloud Development Kit Authors
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

# Harness for Cloud SQL Postgres tests.

terraform {
  required_version = ">= 1.1.0"
  required_providers {
    google = {
      version = "4.40.0"
    }
    random = {
      version = "3.4.3"
    }
  }
}

provider "google" {
  project = var.project
  region  = var.region
}

variable "project" {
  type        = string
  description = "Project ID - Google Cloud project ID in which to create resources."
}

variable "user_email" {
  type        = string
  description = "User email address - Google identity to be used for testing IAM authentication."
}

variable "region" {
  default     = "us-central1"
  description = "GCP region to create database and storage in, for example 'us-central1'. See https://cloud.google.com/compute/docs/regions-zones/ for valid values."
}

locals {
  sql_instance = "go-cloud-test-${random_id.sql_instance.hex}"
}

resource "google_project_service" "sql" {
  service            = "sql-component.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "sqladmin" {
  service            = "sqladmin.googleapis.com"
  disable_on_destroy = false
}

resource "random_id" "sql_instance" {
  keepers = {
    project = var.project
    region  = var.region
  }

  byte_length = 12
}

resource "google_project_iam_member" "cloudsql_client" {
  project = var.project
  role    = "roles/cloudsql.client"
  member  = "user:${var.user_email}"
}

resource "google_project_iam_member" "cloudsql_instanceUser" {
  project = var.project
  role    = "roles/cloudsql.instanceUser"
  member  = "user:${var.user_email}"
}

resource "google_sql_database_instance" "main" {
  name             = local.sql_instance
  database_version = "POSTGRES_9_6"
  region           = var.region
  project          = var.project

  settings {
    tier      = "db-f1-micro"
    disk_size = 10 # GiB
    database_flags {
      name  = "cloudsql.iam_authentication"
      value = "on"
    }
  }

  depends_on = [
    google_project_service.sql,
    google_project_service.sqladmin,
  ]
}

resource "google_sql_database" "main" {
  project  = var.project
  name     = "testdb"
  instance = google_sql_database_instance.main.name
}

resource "random_string" "db_password" {
  keepers = {
    project = var.project
    db_name = local.sql_instance
    region  = var.region
  }

  special = false
  length  = 20
}

resource "google_sql_user" "root" {
  type     = "BUILT_IN"
  name     = "root"
  instance = google_sql_database_instance.main.name
  password = random_string.db_password.result
}

resource "google_sql_user" "user_account" {
  type     = "CLOUD_IAM_USER"
  name     = var.user_email
  instance = google_sql_database_instance.main.name
}

output "project" {
  value       = var.project
  description = "The GCP project ID."
}

output "region" {
  value       = var.region
  description = "The Cloud SQL instance region."
}

output "instance" {
  value       = local.sql_instance
  description = "The Cloud SQL instance region."
}

output "username" {
  value       = "root"
  description = "The Cloud SQL username to connect with."
}

output "password" {
  value       = random_string.db_password.result
  sensitive   = true
  description = "The Cloud SQL instance password for the user."
}

output "database" {
  value       = "testdb"
  description = "The name of the database inside the Cloud SQL instance."
}

output "user_email" {
  value       = var.user_email
  description = "The email of a GCP service account used for testing connections."
}
