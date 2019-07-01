provider "random" {
  version = "~> 2.1"
}

resource "random_string" "gocdk_suffix" {
  special = false
  upper   = false
  length  = 7
}

locals {
  gocdk_random_name = "gocdk-${random_string.gocdk_suffix.result}"
}
