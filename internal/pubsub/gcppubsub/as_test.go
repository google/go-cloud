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

func TestTopicAs(t *testing.T) {
	var c *raw.PublisherClient
	top := gcppubsub.OpenTopic(context.Background(), c, proj, topicName, nil)
	t.Run("can fail", func(t *testing.T) {
		var c2 raw.PublisherClient
		if top.As(&c2) {
			t.Errorf("cast succeeded for %T, want failure", &c2)
		}
	})
	t.Run("can succeed", func(t *testing.T) {
		var c2 *raw.PublisherClient
		if !top.As(&c2) {
			t.Fatalf("cast failed for %T", &c2)
		}
		if c2 != c {
			t.Errorf("got %p, want %p", c2, c)
		}
	})
}

func TestSubscriptionAs(t *testing.T) {
	var c *raw.SubscriberClient
	sub := gcppubsub.OpenSubscription(context.Background(), c, proj, subName, nil)
	t.Run("can fail", func(t *testing.T) {
		var c2 raw.SubscriberClient
		if sub.As(&c2) {
			t.Errorf("cast succeeded for %T, want failure", &c2)
		}
	})
	t.Run("can succeed", func(t *testing.T) {
		var c *raw.SubscriberClient
		sub := gcppubsub.OpenSubscription(context.Background(), c, proj, subName, nil)
		var c2 *raw.SubscriberClient
		if !sub.As(&c2) {
			t.Fatalf("cast failed for %T", &c2)
		}
		if c2 != c {
			t.Errorf("got %p, want %p", c2, c)
		}
	})
}
