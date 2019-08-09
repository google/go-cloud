# Specify the provider and access details
provider "aws" {
  version = "~> 2.0"
  region  = "${var.aws_region}"
}

resource "aws_security_group" "docdbtest" {
  name_prefix = "docdbtest"
  description = "Test mongo driver on docdb"
  vpc_id      = "${var.vpc_id}"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Public SSH access"
  }

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
    description = "Allow traffic within the security group for port forwarding"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# Provisioning a DocumentDB cluster with an instance.

resource "aws_docdb_cluster_instance" "docdbtest" {
  cluster_identifier = "${aws_docdb_cluster.docdbtest.id}"
  identifier_prefix  = "${aws_docdb_cluster.docdbtest.id}"
  instance_class     = "db.r5.large"
  apply_immediately  = true
}

resource "aws_docdb_cluster" "docdbtest" {
  cluster_identifier = "docstore-test-cluster"
  master_username    = "${var.db_username}"
  master_password    = "${var.db_password}"

  db_cluster_parameter_group_name = "docstore-test-pg"
  vpc_security_group_ids          = ["${aws_security_group.docdbtest.id}"]

  skip_final_snapshot = true
}

# Provisioning an EC2 instance within the same VPC group for port forwarding.

resource "aws_key_pair" "docdbtest" {
  key_name_prefix = "docdbtest"
  public_key      = "${var.ssh_public_key}"
}

data "aws_ami" "ubuntu" {
  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-bionic-18.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"]
}

resource "aws_instance" "docdbtest" {
  ami                    = "${data.aws_ami.ubuntu.id}"
  instance_type          = "t2.micro"
  vpc_security_group_ids = ["${aws_security_group.docdbtest.id}"]
  key_name                    = "${aws_key_pair.docdbtest.key_name}"
  associate_public_ip_address = true
}
