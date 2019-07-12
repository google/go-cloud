# This file creates an ECS cluster and its instances. The AWS Console creates
# these resources for you automatically, but AWS does not expose an equivalent
# API.

# Create an ECR Docker repository.
# The name used here will be part of the deployed Docker image name.
resource "aws_ecr_repository" "default" {
  name = local.gocdk_random_name
}

# ECS Cluster

resource "aws_ecs_cluster" "default" {
  name = local.gocdk_random_name
}

resource "random_id" "cluster_name" {
  prefix      = "${local.gocdk_random_name}-"
  byte_length = 8
}

# EC2 Instance Networking

resource "aws_security_group" "ecs_cluster" {
  name_prefix = aws_ecs_cluster.default.name
  description = "Security group for the ${aws_ecs_cluster.default.name} cluster"
  vpc_id      = aws_vpc.ecs_cluster.id

  ingress {
    from_port   = 80
    to_port     = 80
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

resource "aws_vpc" "ecs_cluster" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
}

resource "aws_subnet" "ecs_cluster1" {
  vpc_id                  = aws_vpc.ecs_cluster.id
  cidr_block              = "10.0.0.0/24"
  map_public_ip_on_launch = true
}

resource "aws_subnet" "ecs_cluster2" {
  vpc_id                  = aws_vpc.ecs_cluster.id
  cidr_block              = "10.0.1.0/24"
  map_public_ip_on_launch = true
}

resource "aws_internet_gateway" "ecs_cluster" {
  vpc_id = aws_vpc.ecs_cluster.id
}

resource "aws_route_table" "ecs_cluster" {
  vpc_id = aws_vpc.ecs_cluster.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.ecs_cluster.id
  }
}

resource "aws_route_table_association" "ecs_cluster_subnet1" {
  subnet_id      = aws_subnet.ecs_cluster1.id
  route_table_id = aws_route_table.ecs_cluster.id
}

resource "aws_route_table_association" "ecs_cluster_subnet2" {
  subnet_id      = aws_subnet.ecs_cluster2.id
  route_table_id = aws_route_table.ecs_cluster.id
}

# EC2 Instance Templates

data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amzn-ami-2018.03.v-amazon-ecs-optimized"]
  }
  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

resource "aws_launch_configuration" "ecs_cluster_instance" {
  image_id                    = data.aws_ami.amazon_linux.id
  instance_type               = "t2.micro"
  associate_public_ip_address = true
  security_groups             = [aws_security_group.ecs_cluster.id]
  iam_instance_profile        = aws_iam_instance_profile.ecs_instance.id

  # Scratch space for the instance.
  ebs_block_device {
    device_name = "/dev/xvdcz"
    volume_size = 22 # GiB
    volume_type = "gp2"
  }

  # This is necessary to connect the instances to the ECS cluster. DO NOT EDIT.
  user_data = <<-EOT
    #!/bin/bash
    echo ECS_CLUSTER='${aws_ecs_cluster.default.name}' >> /etc/ecs/ecs.config
    echo ECS_BACKEND_HOST= >> /etc/ecs/ecs.config
  EOT
}

resource "aws_autoscaling_group" "ecs_cluster" {
  launch_configuration = aws_launch_configuration.ecs_cluster_instance.name
  vpc_zone_identifier = [
    aws_subnet.ecs_cluster1.id,
    aws_subnet.ecs_cluster2.id,
  ]

  min_size = 0
  # Increase these numbers to the number of instances you want.
  max_size = 1
  desired_capacity = 1

  tag {
    key = "Name"
    value = "ECS Instance - ${aws_ecs_cluster.default.name}"
    propagate_at_launch = true
  }
  tag {
    key = "Description"
    value = "This instance is part of the Auto Scaling group created by the CDK project's biome"
    propagate_at_launch = true
  }
}

# EC2 Instance IAM role.
# This is needed so that the instances are authenticated to the ECS cluster.

resource "aws_iam_role" "ecs_instance" {
  name_prefix = "ecs-instance-"
  description = "IAM role for EC2 instances part of the ${aws_ecs_cluster.default.name} cluster"
  assume_role_policy = <<-EOF
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

resource "aws_iam_role_policy_attachment" "ecs_instance" {
  role       = aws_iam_role.ecs_instance.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

resource "aws_iam_instance_profile" "ecs_instance" {
  role = aws_iam_role.ecs_instance.name
}
