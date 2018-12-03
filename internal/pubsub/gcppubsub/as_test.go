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
