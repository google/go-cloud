# This file creates an Azure KeyVault and key for use with the
# "secrets/azurekeyvault" driver.

locals {
  # The Go CDK URL for the key: https://gocloud.dev/howto/secrets/#azure.
  azurekeyvault_url = "azurekeyvault://${replace(azurerm_key_vault_key.key.id, "https://", "")}"
}

# The azurerm Terraform provider doesn't have a way to get the ID of the
# current user:
# https://github.com/terraform-providers/terraform-provider-azurerm/issues/3234
# This is a a workaround to get it by shelling out to the "az" CLI.
data "external" "current_azure_user" {
  program = ["az", "ad", "signed-in-user", "show", "--query", "{displayName: displayName,objectId: objectId,objectType: objectType}"]
}

# The key vault to hold the key.
resource "azurerm_key_vault" "keyvault" {
  name                = local.gocdk_random_name
  location            = azurerm_resource_group.resource_group.location
  resource_group_name = azurerm_resource_group.resource_group.name
  tenant_id           = data.azurerm_client_config.current.tenant_id
  sku_name            = "standard"

  access_policy {
    tenant_id = data.azurerm_client_config.current.tenant_id
    object_id = data.external.current_azure_user.result.objectId

    key_permissions = [
      "create",
      "decrypt",
      "delete",
      "encrypt",
      "get",
      "list",
      "update",
    ]
  }
}

# The key itself.
resource "azurerm_key_vault_key" "key" {
  name         = local.gocdk_random_name
  key_vault_id = azurerm_key_vault.keyvault.id
  key_type     = "RSA"
  key_size     = 2048

  key_opts = [
    "decrypt",
    "encrypt",
  ]
}
