# A provider for generating random strings.
# https://www.terraform.io/docs/providers/random/index.html
provider "random" {
  version = "~> 2.1"
}

# Generate a random string that will be used as part of the prefix for
# all resources. Randomness makes collisions with existing resources extremely
# unlikely.
resource "random_string" "gocdk_suffix" {
  special = false
  upper   = false
  length  = 7
}

# A variable to be used as the prefix for all resources created.
locals {
  gocdk_random_name = "gocdk-${random_string.gocdk_suffix.result}"
}

