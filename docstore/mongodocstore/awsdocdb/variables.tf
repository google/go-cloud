variable "aws_region" {
  description = "The AWS region to create docdb cluster and ec2 instance in."
  default     = "us-east-2"
}

variable "vpc_id" {
  description = "The ID of the default VPC used by docdb cluster and ec2 instance."
}

variable "ssh_public_key" {
  description = "A public key line in .ssh/authorized_keys format to use to authenticate to your instance. This must be added to your SSH agent for provisioning to succeed."
}

variable "db_username" {
  description = "The master username to login docdb"
}

variable "db_password" {
  description = "The master password to login docdb"
}
