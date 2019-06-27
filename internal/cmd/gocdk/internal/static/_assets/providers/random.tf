provider "random" {
  version = "~> 1.3"
}

resource "random_string" "gocdk_suffix" {
  special = false
  upper   = false
  length  = 7
}

locals {
  gocdk_random_name = "gocdk-${random_string.gocdk_suffix.result}"
}
