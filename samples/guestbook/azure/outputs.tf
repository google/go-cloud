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

output "storage_account" {
  value       = azurerm_storage_account.guestbook.name
  description = "Name of the Storage Account created to store images."
}

output "storage_container" {
  value       = azurerm_storage_container.guestbook.name
  description = "Name of the storage container created to store images."
}

output "access_key" {
  value       = azurerm_storage_account.guestbook.primary_access_key
  description = "The primary access key for the Storage Account."
}

