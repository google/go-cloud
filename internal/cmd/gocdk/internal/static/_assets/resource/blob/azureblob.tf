# This file creates an Azure Blob for use with the "blob/azblob" driver.

locals {
  # The Go CDK URL for the bucket: https://gocloud.dev/howto/blob/#azure.
  azureblob_bucket_url = "azblob://${azurerm_storage_container.bucket.name}"
}

# An Azure storage account to hold the bucket.
resource "azurerm_storage_account" "storage_account" {
  # The storage account name can't have "-" in it.
  name                     = replace(local.gocdk_random_name, "-", "")
  resource_group_name      = azurerm_resource_group.resource_group.name
  location                 = azurerm_resource_group.resource_group.location
  account_tier             = "Standard"
  account_replication_type = "GRS"
}

# The actual bucket.
resource "azurerm_storage_container" "bucket" {
  name                 = local.gocdk_random_name
  resource_group_name  = azurerm_resource_group.resource_group.name
  storage_account_name = azurerm_storage_account.storage_account.name
}

