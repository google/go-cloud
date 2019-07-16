# A provider for Microsoft Azure's Resource Manager.
# https://www.terraform.io/docs/providers/azurerm/index.html
provider "azurerm" {
  version = "~> 1.31.0"
}

# A provider for Microsoft Azure's Active Directory.
# https://www.terraform.io/docs/providers/azuread/index.html
provider "azuread" {
  version = "~> 0.4"
}

# An Azure ResourceGroup.
# https://www.terraform.io/docs/providers/azurerm/r/resource_group.html
resource "azurerm_resource_group" "resource_group" {
  name     = local.gocdk_random_name
  location = local.azure_location
}

# A datasource that accesses the current configuration of the client.
# https://www.terraform.io/docs/providers/azurerm/d/client_config.html
data "azurerm_client_config" "current" {}

