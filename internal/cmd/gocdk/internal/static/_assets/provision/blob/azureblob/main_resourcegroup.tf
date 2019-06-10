resource "azurerm_resource_group" "resource_group" {
  name     = "${random_string.azure_suffix.result}"
  location = "${local.azure_location}"
}

