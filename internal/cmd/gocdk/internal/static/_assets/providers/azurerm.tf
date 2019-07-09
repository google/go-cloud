provider "azurerm" {
  version = "~> 1.31.0"
}

provider "azuread" {
  version = "~> 0.4"
}

resource "azurerm_resource_group" "resource_group" {
  name     = local.gocdk_random_name
  location = local.azure_location
}

data "azurerm_client_config" "current" {}
