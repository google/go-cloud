# A provider for Amazon Web Services (AWS).
# https://www.terraform.io/docs/providers/aws/index.html
provider "aws" {
  version = "~> 2.0"
  region  = local.aws_region
}

