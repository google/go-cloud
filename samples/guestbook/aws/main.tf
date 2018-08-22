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

provider "aws" {
  version = "~> 1.22"
  region  = "${var.region}"
}

provider "random" {
  version = "~> 1.3"
}

# Firewalls

resource "aws_security_group" "guestbook" {
  name_prefix = "guestbook"
  description = "Sandbox for the Guestbook Go Cloud sample app."

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Public SSH access"
  }

  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Public HTTP access"
  }

  ingress {
    from_port   = 3306
    to_port     = 3306
    protocol    = "tcp"
    self        = true
    description = "MySQL within group"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "All outgoing traffic allowed"
  }
}

# SQL Database (RDS)

resource "random_string" "db_password" {
  special = false
  length  = 20
}

resource "aws_db_instance" "guestbook" {
  identifier_prefix      = "guestbook"
  engine                 = "mysql"
  engine_version         = "5.6.39"
  instance_class         = "db.t2.micro"
  allocated_storage      = 20
  username               = "root"
  password               = "${random_string.db_password.result}"
  name                   = "guestbook"
  publicly_accessible    = true
  vpc_security_group_ids = ["${aws_security_group.guestbook.id}"]
  skip_final_snapshot    = true

  provisioner "local-exec" {
    # TODO(light): Reuse credentials from Terraform.
    command = "go run '${path.module}'/provision_db/main.go -host='${aws_db_instance.guestbook.address}' -security_group='${aws_security_group.guestbook.id}' -database=guestbook -password='${random_string.db_password.result}' -schema='${path.module}'/../schema.sql"
  }
}

# Blob Storage (S3)

resource "aws_s3_bucket" "guestbook" {
  bucket_prefix = "guestbook"
}

resource "aws_s3_bucket_object" "aws" {
  bucket = "${aws_s3_bucket.guestbook.bucket}"
  key = "aws.png"
  content_type = "image/png"
  source = "${path.module}/../blobs/aws.png"
}

resource "aws_s3_bucket_object" "gcp" {
  bucket = "${aws_s3_bucket.guestbook.bucket}"
  key = "gcp.png"
  content_type = "image/png"
  source = "${path.module}/../blobs/gcp.png"
}

resource "aws_s3_bucket_object" "gophers" {
  bucket = "${aws_s3_bucket.guestbook.bucket}"
  key = "gophers.jpg"
  content_type = "image/jpeg"
  source = "${path.module}/../blobs/gophers.jpg"
}

# Paramstore (SSM)

resource "aws_ssm_parameter" "motd" {
  name  = "${var.paramstore_var}"
  type  = "String"
  value = "ohai from AWS"
}

# Compute (EC2)

resource "aws_iam_role" "guestbook" {
  name_prefix = "guestbook"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": {
    "Effect": "Allow",
    "Principal": {"Service": "ec2.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }
}
EOF
}

resource "aws_iam_role_policy" "guestbook" {
  name_prefix = "Guestbook-Policy"
  role        = "${aws_iam_role.guestbook.id}"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": {
    "Effect": "Allow",
    "Action": [
        "s3:GetObject",
        "ssm:DescribeParameters",
        "ssm:GetParameter",
        "ssm:GetParameters",
        "xray:PutTraceSegments",
        "xray:PutTelemetryRecords"
    ],
    "Resource": "*"
  }
}
EOF
}

resource "aws_iam_instance_profile" "guestbook" {
  name_prefix = "guestbook"
  role        = "${aws_iam_role.guestbook.name}"
}

data "aws_ami" "debian" {
  most_recent = true

  filter {
    name   = "product-code"
    values = ["55q52qvgjfpdj2fpfy9mb1lo4"]
  }

  filter {
    name   = "product-code.type"
    values = ["marketplace"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }

  owners = ["679593333241"]
}

resource "aws_key_pair" "guestbook" {
  key_name_prefix = "guestbook"
  public_key      = "${var.ssh_public_key}"
}

resource "aws_instance" "guestbook" {
  ami                    = "${data.aws_ami.debian.id}"
  instance_type          = "t2.micro"
  vpc_security_group_ids = ["${aws_security_group.guestbook.id}"]
  iam_instance_profile   = "${aws_iam_instance_profile.guestbook.id}"
  key_name               = "${aws_key_pair.guestbook.key_name}"

  connection {
    type = "ssh"
    user = "admin"
  }

  provisioner "file" {
    source      = "${path.module}/../guestbook"
    destination = "/home/admin/guestbook"
  }

  provisioner "remote-exec" {
    inline = ["chmod +x /home/admin/guestbook"]
  }
}
