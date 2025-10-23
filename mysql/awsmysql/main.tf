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

# Harness for MySQL tests.

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
  description = "Security group for the Go CDK MySQL test database."

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Public MySQL access"
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
  iamUser = "iam-user"
}

resource "aws_db_instance" "main" {
  identifier_prefix      = "go-cloud-test"
  engine                 = "mysql"
  engine_version         = "8.0.43"
  instance_class         = "db.t3.micro"
  allocated_storage      = 20
  username               = "root"
  password               = random_string.db_password.result
  db_name                = "testdb"
  publicly_accessible    = true
  vpc_security_group_ids = [aws_security_group.main.id]
  skip_final_snapshot    = true

  iam_database_authentication_enabled = true
}

# Data source to get current AWS account information
data "aws_caller_identity" "current" {}

# IAM Role that can be assumed to connect to the database
resource "aws_iam_role" "rds_connect" {
  name_prefix = "rds-connect-"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          AWS = data.aws_caller_identity.current.arn
        }
      }
    ]
  })
}

# IAM Policy to allow RDS connection using IAM authentication
resource "aws_iam_policy" "rds_connect" {
  name_prefix = "rds-connect-"
  description = "Allow connecting to RDS MySQL instance using IAM authentication"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "rds-db:connect"
        ]
        Resource = format("arn:aws:rds-db:%s:%s:dbuser:%s/%s",
          var.region, data.aws_caller_identity.current.account_id,
          aws_db_instance.main.resource_id,
          local.iamUser,
        )
      }
    ]
  })
}

# Attach the policy to the role
resource "aws_iam_role_policy_attachment" "rds_connect" {
  role       = aws_iam_role.rds_connect.name
  policy_arn = aws_iam_policy.rds_connect.arn
}

# Automatically create the IAM database user
resource "null_resource" "create_iam_user" {
  depends_on = [aws_db_instance.main]

  provisioner "local-exec" {
    command = <<-EOT
      docker run --rm mysql:8.0 \
        mysql -h ${split(":", aws_db_instance.main.endpoint)[0]} \
              -P ${split(":", aws_db_instance.main.endpoint)[1]} \
              -u ${aws_db_instance.main.username} \
              -p${random_string.db_password.result} \
              -e "CREATE USER IF NOT EXISTS '${local.iamUser}' IDENTIFIED WITH AWSAuthenticationPlugin AS 'RDS'; GRANT ALL PRIVILEGES ON ${aws_db_instance.main.db_name}.* TO 'iam-user'@'%'; FLUSH PRIVILEGES;"
    EOT
  }

  triggers = {
    endpoint = aws_db_instance.main.endpoint
  }
}

output "endpoint" {
  value       = aws_db_instance.main.endpoint
  description = "The RDS instance's host/port."
}

output "username" {
  value       = "root"
  description = "The MySQL username to connect with."
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

output "iam_role_arn" {
  value       = aws_iam_role.rds_connect.arn
  description = "The ARN of the IAM role that can connect to the database"
}

output "iam_db_username" {
  value       = local.iamUser
  description = "The IAM database username to use for IAM authentication"
}
