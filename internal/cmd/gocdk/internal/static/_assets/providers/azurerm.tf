provider "azurerm" {
  version = "~> 1.27.0"
}

resource "azurerm_resource_group" "resource_group" {
  name     = "${local.gocdk_random_name}"
  location = "${local.azure_location}"
}
