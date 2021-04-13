// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rabbitpubsub provides an pubsub implementation for RabbitMQ.
// Use OpenTopic to construct a *pubsub.Topic, and/or OpenSubscription
// to construct a *pubsub.Subscription.
//
// RabbitMQ follows the AMQP specification, which uses different terminology
// than the Go CDK Pub/Sub.
//
// A Pub/Sub topic is an AMQP exchange. The exchange kind should be "fanout" to match
// the Pub/Sub model, although publishing will work with any kind of exchange.
//
// A Pub/Sub subscription is an AMQP queue. The queue should be bound to the exchange
// that is the topic of the subscription. See the package example for details.
//
// URLs
//
// For pubsub.OpenTopic and pubsub.OpenSubscription, rabbitpubsub registers
// for the scheme "rabbit".
// The default URL opener will connect to a default server based on the
// environment variable "RABBIT_SERVER_URL".
// To customize the URL opener, or for more details on the URL format,
// see URLOpener.
// See https://gocloud.dev/concepts/urls/ for background information.
//
// Message Delivery Semantics
//
// RabbitMQ supports at-least-once semantics; applications must
// call Message.Ack after processing a message, or it will be redelivered.
// See https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more background.
//
// As
//
// rabbitpubsub exposes the following types for As:
//  - Topic: *amqp.Connection
//  - Subscription: *amqp.Connection
//  - Message.BeforeSend: *amqp.Publishing
//  - Message.AfterSend: None
//  - Message: amqp.Delivery
//  - Error: *amqp.Error and MultiError
package rabbitpubsub // import "gocloud.dev/pubsub/rabbitpubsub"
