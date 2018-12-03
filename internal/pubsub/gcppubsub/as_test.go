package gcppubsub_test

import (
	"context"
	"testing"

	raw "cloud.google.com/go/pubsub/apiv1"
	"github.com/google/go-cloud/internal/pubsub/gcppubsub"
)

func TestTopicAsCanFail(t *testing.T) {
	proj := ""
	topicName := "some-topic"
	top := gcppubsub.OpenTopic(context.Background(), client, proj, topicName, nil)
	var c raw.PublisherClient
	if top.As(&c) {
		t.Errorf("cast succeeded for %T, want failure", &c)
	}
}

func TestTopicAsCanSucceed(t *testing.T) {
	proj := ""
	topicName := "some-topic"
	top := gcppubsub.OpenTopic(context.Background(), client, proj, topicName, nil)
	var c *raw.PublisherClient
	if !top.As(&c) {
		t.Errorf("cast failed for %T", &c)
	}
}

func TestSubscriptionAsCanFail(t *testing.T) {
	proj := ""
	subName := "some-sub"
	sub := gcppubsub.OpenSubscription(context.Background(), client, proj, subName, nil)
	var c raw.SubscriptionClient
	if top.As(&c) {
		t.Errorf("cast succeeded for %T, want failure", &c)
	}
}

func TestSubscriptionAsCanSucceed(t *testing.T) {
	proj := ""
	subName := "some-sub"
	top := gcppubsub.OpenSubscription(context.Background(), client, proj, subName, nil)
	var c *raw.SubscriptionClient
	if !top.As(&c) {
		t.Errorf("cast failed for %T", &c)
	}
}
