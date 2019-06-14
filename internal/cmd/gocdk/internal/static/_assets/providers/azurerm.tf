provider "azurerm" {
  version = "~> 1.27.0"
}

resource "random_string" "azure_suffix" {
  special = false
  upper = false
  length  = 7
}

resource "azurerm_resource_group" "resource_group" {
  name     = "${random_string.azure_suffix.result}"
  location = "${local.azure_location}"
}
