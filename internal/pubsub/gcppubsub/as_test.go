package gcppubsub_test

import (
	"context"
	"testing"

	raw "cloud.google.com/go/pubsub/apiv1"
	"github.com/google/go-cloud/gcp"
	"github.com/google/go-cloud/internal/pubsub/gcppubsub"
)

var proj gcp.ProjectID
var topicName = "some-topic"
var subName = "some-sub"

func TestTopicAsCanFail(t *testing.T) {
	var client *raw.PublisherClient
	top := gcppubsub.OpenTopic(context.Background(), client, proj, topicName, nil)
	var c raw.PublisherClient
	if top.As(&c) {
		t.Errorf("cast succeeded for %T, want failure", &c)
	}
}

func TestTopicAsCanSucceed(t *testing.T) {
	var client *raw.PublisherClient
	top := gcppubsub.OpenTopic(context.Background(), client, proj, topicName, nil)
	var c *raw.PublisherClient
	if !top.As(&c) {
		t.Errorf("cast failed for %T", &c)
	}
}

func TestSubscriptionAsCanFail(t *testing.T) {
	var client *raw.SubscriberClient
	sub := gcppubsub.OpenSubscription(context.Background(), client, proj, subName, nil)
	var c raw.SubscriberClient
	if sub.As(&c) {
		t.Errorf("cast succeeded for %T, want failure", &c)
	}
}

func TestSubscriptionAsCanSucceed(t *testing.T) {
	var client *raw.SubscriberClient
	sub := gcppubsub.OpenSubscription(context.Background(), client, proj, subName, nil)
	var c *raw.SubscriberClient
	if !sub.As(&c) {
		t.Errorf("cast failed for %T", &c)
	}
}
