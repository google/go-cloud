variable "region" {
  type        = "string"
  description = "Region to create resources in. See https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html for valid values."
}

variable "ssh_public_key" {
  type        = "string"
  description = "A public key line in .ssh/authorized_keys format to use to authenticate to your instance. This must be added to your SSH agent for provisioning to succeed."
}

variable "app_binary" {
  type        = "string"
  description = "The path to the test app binary to be copied to EC2."
}

variable "gcp_project" {
  type        = "string"
  description = "The project ID of the GCP project used for Stackdriver logging."
}

variable "stackdriver_service_account" {
  type        = "string"
  description = "The username part of the service account email to access Stackdriver"
}

variable "aws_credentials_file" {
  type        = "string"
  description = "The path to the AWS credentials file."
}

variable "gcp_service_account_file" {
  type        = "string"
  description = "The service account files used to get ADC"
}
