# This file creates an Azure CosmoDB database for use with the
# "docstore/mongodocstore" driver.

locals {
  # The Go CDK URL for the database: https://gocloud.dev/howto/docstore/#cosmos.
  azurecosmos_url = "mongo://demo-db/demo-collection?id_field=Key"
  # The MONGO_SERVER_URL to be used to connect to the CosmosDB using a Mongo client.
  azurecosmos_mongo_url = azurerm_cosmosdb_account.account.connection_strings[0]
}

# A CosmosDB account.
# Databases and collections are created automatically within the account, so
# there's no need to provision them explicitly.
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

