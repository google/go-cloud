# TODO(rvangent): Add comments explaining.

locals {
  azuresb_topic_url        = "azuresb://${azurerm_servicebus_topic.topic.name}"
  azuresb_subscription_url = "azuresb://${azurerm_servicebus_topic.topic.name}?subscription=${azurerm_servicebus_subscription.subscription.name}"
}

resource "azurerm_servicebus_namespace" "namespace" {
  name                = local.gocdk_random_name
  location            = azurerm_resource_group.resource_group.location
  resource_group_name = azurerm_resource_group.resource_group.name
  sku                 = "Standard"
}

resource "azurerm_servicebus_topic" "topic" {
  name                = local.gocdk_random_name
  resource_group_name = azurerm_resource_group.resource_group.name
  namespace_name      = azurerm_servicebus_namespace.namespace.name

  enable_partitioning = true
}

resource "azurerm_servicebus_subscription" "subscription" {
  name                = local.gocdk_random_name
  resource_group_name = azurerm_resource_group.resource_group.name
  namespace_name      = azurerm_servicebus_namespace.namespace.name
  topic_name          = azurerm_servicebus_topic.topic.name
  max_delivery_count  = 1
}

