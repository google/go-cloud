terraform {
  required_version = "~>0.12"
}

# See documentation for more info: https://www.terraform.io/docs/providers/azurerm/auth/azure_cli.html
provider "azurerm" {
  version = "~> 1.22"
}

provider "random" {
  version = "~> 2.1"
}

# Run Azure CLI command "az account list-locations" to see list of all locations.
variable "location" {
  description = "The Azure Region in which all resources in this example should be created."
}

# See documentation for more info: https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-overview
variable "resourcegroup" {
  description = "The Azure Resource Group Name within your Subscription in which this resource will be created."
}

resource "random_string" "db_password" {
  keepers = {
    region = var.location
  }

  special = false
  length  = 20
}

resource "random_id" "serverid" {
  keepers = {
    region = var.location
  }

  byte_length = 2
}

resource "azurerm_resource_group" "mysqlrg" {
  name     = var.resourcegroup
  location = var.location
}

resource "azurerm_mysql_server" "mysqlserver" {
  name                = format("go-cdk-test-%v", random_id.serverid.dec)
  location            = azurerm_resource_group.mysqlrg.location
  resource_group_name = azurerm_resource_group.mysqlrg.name

  sku {
    name     = "B_Gen5_2"
    capacity = 2
    tier     = "Basic"
    family   = "Gen5"
  }

  storage_profile {
    storage_mb            = 5120
    backup_retention_days = 7
    geo_redundant_backup  = "Disabled"
  }

  administrator_login          = "gocloudadmin"
  administrator_login_password = random_string.db_password.result
  version                      = "5.7"
  ssl_enforcement              = "Enabled"
}

# See documentation for more info: https://www.terraform.io/docs/providers/azurerm/r/sql_firewall_rule.html
resource "azurerm_mysql_firewall_rule" "addrule" {
  name                = "ClientIPAddress"
  resource_group_name = azurerm_resource_group.mysqlrg.name
  server_name         = azurerm_mysql_server.mysqlserver.name
  start_ip_address    = "0.0.0.0"
  end_ip_address      = "255.255.255.255"
}

resource "azurerm_mysql_database" "mysqldb" {
  name                = "testdb"
  resource_group_name = azurerm_resource_group.mysqlrg.name
  server_name         = azurerm_mysql_server.mysqlserver.name
  charset             = "utf8"
  collation           = "utf8_unicode_ci"
}

output "username" {
  value       = "${azurerm_mysql_server.mysqlserver.administrator_login}@${azurerm_mysql_server.mysqlserver.name}"
  description = "The MySQL username to connect with."
}

output "password" {
  value       = random_string.db_password.result
  sensitive   = true
  description = "The MySQL instance password for the user."
}

output "servername" {
  value       = azurerm_mysql_server.mysqlserver.fqdn
  description = "The host name of the Azure Database for MySQL instance."
}

output "database" {
  value       = "testdb"
  description = "The databasename of the Azure Database for MySQL instance."
}

