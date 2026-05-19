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
  required_version = "~>1.13.2"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source = "hashicorp/random"
    }
  }
}

provider "aws" {
  region = var.region
}

provider "random" {
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

locals {
  iamUser = "iamuser"
}

resource "aws_db_instance" "main" {
  identifier_prefix               = "go-cloud-test"
  engine                          = "postgres"
  engine_version                  = "16.6"
  instance_class                  = "db.t3.micro"
  allocated_storage               = 20
  username                        = "root"
  password                        = random_string.db_password.result
  db_name                         = "testdb"
  publicly_accessible             = true
  vpc_security_group_ids          = [aws_security_group.main.id]
  skip_final_snapshot             = true
  parameter_group_name            = aws_db_parameter_group.main.name
  iam_database_authentication_enabled = true
}

resource "aws_db_parameter_group" "main" {
  name_prefix = "go-cloud-test"
  family      = "postgres16"

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

# IAM authentication resources.

data "aws_caller_identity" "current" {}

resource "aws_iam_policy" "rds_iam_auth" {
  name_prefix = "go-cloud-test-rds-iam"
  description = "Allow IAM authentication to the RDS PostgreSQL test instance."

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = "rds-db:connect"
        Resource = format("arn:aws:rds-db:%s:%s:dbuser:%s/%s",
          var.region, data.aws_caller_identity.current.account_id,
          aws_db_instance.main.resource_id,
          local.iamUser,
        )
      }
    ]
  })
}

resource "aws_iam_role" "rds_iam_auth" {
  name_prefix = "go-cloud-test-rds-iam"
  description = "Role for IAM authentication to the RDS PostgreSQL test instance."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          AWS = data.aws_caller_identity.current.arn
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "rds_iam_auth" {
  role       = aws_iam_role.rds_iam_auth.name
  policy_arn = aws_iam_policy.rds_iam_auth.arn
}

# Automatically create the IAM database user in PostgreSQL.
resource "null_resource" "create_iam_user" {
  depends_on = [aws_db_instance.main]

  provisioner "local-exec" {
    command = <<-EOT
      docker run --rm postgres:16 \
        psql "host=${split(":", aws_db_instance.main.endpoint)[0]} port=${split(":", aws_db_instance.main.endpoint)[1]} dbname=testdb user=${aws_db_instance.main.username} password=${random_string.db_password.result} sslmode=require" \
        -c "CREATE USER ${local.iamUser}; GRANT rds_iam TO ${local.iamUser}; GRANT ALL ON SCHEMA public TO ${local.iamUser};"
    EOT
  }

  triggers = {
    endpoint = aws_db_instance.main.endpoint
  }
}

output "iam_username" {
  value       = local.iamUser
  description = "The PostgreSQL username for IAM authentication."
}

output "iam_role_arn" {
  value       = aws_iam_role.rds_iam_auth.arn
  description = "The ARN of the IAM role to assume for IAM database authentication."
}

