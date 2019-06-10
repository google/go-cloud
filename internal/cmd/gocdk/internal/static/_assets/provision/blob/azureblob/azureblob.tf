# TODO(rvangent): Add comments explaining.

locals {
  azureblob_bucket_url = "azblob://${azurerm_storage_container.bucket.name}"
}

resource "azurerm_storage_account" "storage_account" {
  name                     = "gocdk${random_string.azure_suffix.result}"
  resource_group_name      = "${azurerm_resource_group.resource_group.name}"
  location                 = "${azurerm_resource_group.resource_group.location}"
  account_tier             = "Standard"
  account_replication_type = "GRS"
}

resource "azurerm_storage_container" "bucket" {
  name                  = "gocdk-${random_string.azure_suffix.result}"
  resource_group_name   = "${azurerm_resource_group.resource_group.name}"
  storage_account_name  = "${azurerm_storage_account.storage_account.name}"
}


