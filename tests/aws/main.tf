provider "aws" {
  region = "${var.region}"
}

# Firewalls

resource "aws_security_group" "app" {
  name_prefix = "go-cloud-test-app"
  description = "Sandbox for the Go X Cloud test app."

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
    source      = "${var.aws_credentials_file}"
    destination = "/home/admin/aws-creds"
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
