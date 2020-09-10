# Copyright 2020 The Go Cloud Development Kit Authors
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

# Harness for MySQL tests.

terraform {
  required_version = ">= 0.13"
  required_providers {
    docker = {
      source  = "terraform-providers/docker"
      version = "~> 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 2.0"
    }
  }
}

variable port {
  type        = number
  description = "Port exposed out of the MySQL container."
  default     = 3306
}

resource random_pet mysql {}

resource random_password db_password {
  special = false
  length  = 20
}

locals {
  db_name = "testdb"
}

resource docker_container mysql {
  name  = random_pet.mysql.id
  image = docker_image.mysql.latest

  env = [
    "MYSQL_ROOT_PASSWORD=${random_password.db_password.result}",
    "MYSQL_DATABASE=${local.db_name}",
  ]
  ports {
    internal = 3306
    external = var.port
  }
}

resource docker_image mysql {
  name = "mysql"
}

output endpoint {
  value       = "localhost:${var.port}"
  description = "The MySQL instance's host/port."
}

output username {
  value       = "root"
  description = "The MySQL username to connect with."
}

output password {
  value       = random_password.db_password.result
  sensitive   = true
  description = "The MySQL instance password for the user."
}

output database {
  value       = local.db_name
  description = "The name of the database inside the MySQL instance."
}

