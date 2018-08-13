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

provider "google" {
  version = "~> 1.15"
  project = "${var.gcp_project}"
}

# Firewalls

resource "aws_security_group" "app" {
  name_prefix = "go-cloud-test-app"
  description = "Sandbox for the Go Cloud test app."

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

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "All outgoing traffic allowed"
  }
}

# GCP service account for accessing Stackdriver logging

resource "google_service_account" "sd_project" {
  account_id   = "${var.stackdriver_service_account}"
  display_name = "Stackdriver project"
}

resource "google_service_account_key" "sd_project" {
  service_account_id = "${google_service_account.sd_project.name}"
}

resource "google_project_service" "logging" {
  service            = "logging.googleapis.com"
  disable_on_destroy = false
}

# EC2 setup for AWS

resource "aws_iam_role" "app" {
  name_prefix = "go-cloud-test-app"

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

resource "aws_iam_role_policy" "app" {
  name_prefix = "App-Policy"
  role        = "${aws_iam_role.app.id}"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": {
    "Effect": "Allow",
    "Action": [
      "xray:PutTelemetryRecords",
      "xray:PutTraceSegments"
    ],
    "Resource": "*"
  }
}
EOF
}

resource "aws_iam_instance_profile" "app" {
  name_prefix = "go-cloud-test-app"
  role        = "${aws_iam_role.app.name}"
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

resource "aws_instance" "app" {
  ami                    = "${data.aws_ami.debian.id}"
  instance_type          = "t2.micro"
  vpc_security_group_ids = ["${aws_security_group.app.id}"]
  iam_instance_profile   = "${aws_iam_instance_profile.app.id}"
  key_name               = "${aws_key_pair.app.key_name}"

  connection {
    type = "ssh"
    user = "admin"
  }

  provisioner "file" {
    source      = "${var.app_binary}"
    destination = "/home/admin/app"
  }

  provisioner "file" {
    source      = "${var.gcp_service_account_file}"
    destination = "/home/admin/gcp-adc.json"
  }

  provisioner "remote-exec" {
    inline = [
      "sudo mkdir -p /etc/google/auth",
      "sudo mv '/home/admin/gcp-adc.json' '/etc/google/auth/application_default_credentials.json'",
      "curl -sSO https://dl.google.com/cloudagents/install-logging-agent.sh",
      "sudo bash install-logging-agent.sh",
      "chmod +x /home/admin/app",
    ]
  }
}

resource "aws_key_pair" "app" {
  key_name_prefix = "app"
  public_key      = "${var.ssh_public_key}"
}
