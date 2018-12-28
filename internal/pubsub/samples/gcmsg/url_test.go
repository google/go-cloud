package main

import (
	"fmt"
	"testing"
)

func TestParseTopicURL(t *testing.T) {
	tcs := []struct {
		input string
		want  URL
	}{
		{"gcppubsub://projects/go-cloud-test-216917/topics/test-topic", URL{Provider: "gcp", Project: "go-cloud-test-216917", Topic: "test-topic"}},
		{"rabbitpubsub://guest:guest@localhost:5672/topics/test-topic", URL{Provider: "rabbit", ServerURL: "amqp://guest:guest@localhost:5672", Topic: "test-topic"}},
	}
	for _, tc := range tcs {
		t.Run(tc.input, func(t *testing.T) {
			u, err := parseTopicURL(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if u != tc.want {
				t.Errorf("got %+v, want %+v", u, tc.want)
			}
		})
	}

	badInputs := []string{"", "a", "http://www.foo.com"}
	for _, bi := range badInputs {
		t.Run(fmt.Sprintf("bad input: %q", bi), func(t *testing.T) {
			_, err := parseTopicURL(bi)
			if err == nil {
				t.Error("got no error")
			}
		})
	}
}

func TestParseSubscriptionURL(t *testing.T) {
	tcs := []struct {
		input string
		want  URL
	}{
		{"gcppubsub://projects/go-cloud-test-216917/subscriptions/test-subscription-1", URL{Provider: "gcp", Project: "go-cloud-test-216917", Subscription: "test-subscription-1"}},
		{"rabbitpubsub://guest:guest@localhost:5672/subscriptions/test-subscription-1", URL{Provider: "rabbit", ServerURL: "amqp://guest:guest@localhost:5672", Subscription: "test-subscription-1"}},
	}
	for _, tc := range tcs {
		t.Run(tc.input, func(t *testing.T) {
			u, err := parseSubscriptionURL(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if u != tc.want {
				t.Errorf("got %+v, want %+v", u, tc.want)
			}
		})
	}

	badInputs := []string{"", "a", "http://www.foo.com"}
	for _, bi := range badInputs {
		t.Run(fmt.Sprintf("bad input: %q", bi), func(t *testing.T) {
			_, err := parseSubscriptionURL(bi)
			if err == nil {
				t.Error("got no error")
			}
		})
	}
}
