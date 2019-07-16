# This file creates a Azure Service Bus topic and a subscription subscribed to
# it, for use with the "pubsub/azuresb" driver.

locals {
  # The Go CDK URL for the topic: https://gocloud.dev/howto/pubsub/publish#azure.
  azuresb_topic_url = "azuresb://${azurerm_servicebus_topic.topic.name}"
  # The Go CDK URL for the subscription: https://gocloud.dev/howto/pubsub/subscribe#azure.
  azuresb_subscription_url = "azuresb://${azurerm_servicebus_topic.topic.name}?subscription=${azurerm_servicebus_subscription.subscription.name}"
}

# A ServiceBus namespace to hold the topic and subscription.
resource "azurerm_servicebus_namespace" "namespace" {
  name                = local.gocdk_random_name
  location            = azurerm_resource_group.resource_group.location
  resource_group_name = azurerm_resource_group.resource_group.name
  sku                 = "Standard"
}

# The topic.
resource "azurerm_servicebus_topic" "topic" {
  name                = local.gocdk_random_name
  resource_group_name = azurerm_resource_group.resource_group.name
  namespace_name      = azurerm_servicebus_namespace.namespace.name

  enable_partitioning = true
}

# The subscription, subscribed to the topic.
resource "azurerm_servicebus_subscription" "subscription" {
  name                = local.gocdk_random_name
  resource_group_name = azurerm_resource_group.resource_group.name
  namespace_name      = azurerm_servicebus_namespace.namespace.name
  topic_name          = azurerm_servicebus_topic.topic.name
  max_delivery_count  = 1
}

