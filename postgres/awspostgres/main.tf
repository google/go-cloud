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

# Harness for RDS PostgreSQL tests.

terraform {
  required_version = "~>0.12"
}

provider "aws" {
  version = "~> 2.7"
  region  = var.region
}

provider "random" {
  version = "~> 2.1"
}

variable "region" {
  type        = string
  description = "Region to create resources in. See https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html for valid values."
}

resource "aws_security_group" "main" {
  name_prefix = "testdb"
  description = "Security group for the Go CDK Postgres test database."

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Public Postgres access"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "All outgoing traffic allowed"
  }
}

resource "random_string" "db_password" {
  keepers = {
    region = var.region
  }

  special = false
  length  = 20
}

resource "aws_db_instance" "main" {
  identifier_prefix      = "go-cloud-test"
  engine                 = "postgres"
  engine_version         = "10.5"
  instance_class         = "db.t2.micro"
  allocated_storage      = 20
  username               = "root"
  password               = random_string.db_password.result
  name                   = "testdb"
  publicly_accessible    = true
  vpc_security_group_ids = [aws_security_group.main.id]
  skip_final_snapshot    = true
  parameter_group_name   = aws_db_parameter_group.main.name
}

resource "aws_db_parameter_group" "main" {
  name_prefix = "go-cloud-test"
  family      = "postgres10"

  parameter {
    name  = "rds.force_ssl"
    value = "1"
  }
}

output "endpoint" {
  value       = aws_db_instance.main.endpoint
  description = "The RDS instance's host/port."
}

output "username" {
  value       = "root"
  description = "The PostgreSQL username to connect with."
}

output "password" {
  value       = random_string.db_password.result
  sensitive   = true
  description = "The RDS instance password for the user."
}

output "database" {
  value       = "testdb"
  description = "The name of the database inside the RDS instance."
}

