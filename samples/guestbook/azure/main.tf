# Copyright 2019 The Go Cloud Development Kit Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

terraform {
  required_version = "~>0.12"
}

provider "azurerm" {
  version = "~> 1.27.0"
}

provider "random" {
  version = "~> 2.1"
}

resource "random_string" "suffix" {
  special = false
  upper   = false
  length  = 7
}

# Create a resource group
resource "azurerm_resource_group" "guestbook" {
  name     = "guestbook${random_string.suffix.result}"
  location = var.location
}

# Create a Storage Account, container, and two blobs.

resource "azurerm_storage_account" "guestbook" {
  name                     = "guestbook${random_string.suffix.result}"
  resource_group_name      = azurerm_resource_group.guestbook.name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "GRS"
}

resource "azurerm_storage_container" "guestbook" {
  name                  = "guestbook${random_string.suffix.result}"
  resource_group_name   = azurerm_resource_group.guestbook.name
  storage_account_name  = azurerm_storage_account.guestbook.name
  container_access_type = "private"
}

resource "azurerm_storage_blob" "gopher" {
  name = "azure.png"

  resource_group_name    = azurerm_resource_group.guestbook.name
  storage_account_name   = azurerm_storage_account.guestbook.name
  storage_container_name = azurerm_storage_container.guestbook.name

  type         = "block"
  content_type = "image/png"
  source       = "${path.module}/../blobs/azure.png"
}

resource "azurerm_storage_blob" "motd" {
  name = "motd"

  resource_group_name    = azurerm_resource_group.guestbook.name
  storage_account_name   = azurerm_storage_account.guestbook.name
  storage_container_name = azurerm_storage_container.guestbook.name

  type         = "block"
  content_type = "text/plain"
  source       = "${path.module}/../blobs/motd.txt"
}

