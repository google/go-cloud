# TODO(rvangent): Add comments explaining.

locals {
  azurecosmos_url       = "mongo://demo-db/demo-collection?id_field=Key"
  azurecosmos_mongo_url = azurerm_cosmosdb_account.account.connection_strings[0]
}

resource "azurerm_cosmosdb_account" "account" {
  name                = local.gocdk_random_name
  location            = azurerm_resource_group.resource_group.location
  resource_group_name = azurerm_resource_group.resource_group.name
  offer_type          = "Standard"
  kind                = "MongoDB"

  consistency_policy {
    consistency_level = "Session"
  }

  geo_location {
    location          = azurerm_resource_group.resource_group.location
    failover_priority = 0
  }
}

