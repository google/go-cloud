provider "azurerm" {
  version = "~> 1.21.0"
}

resource "azurerm_resource_group" "resource_group" {
  # The name can't have "-" in it.
  name     = "${replace(local.gocdk_random_name, "-", "")}"
  location = "${local.azure_location}"
}
