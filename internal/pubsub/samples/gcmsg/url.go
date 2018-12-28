package main

import (
	"context"
	"fmt"
	"regexp"

	"gocloud.dev/internal/pubsub"
)

// URL contains the parse result of a URL for a topic or subscription.
type URL struct {
	// Provider is "rabbit" or "gcp".
	Provider     string
	Project      string
	ServerURL    string
	Topic        string
	Subscription string
}

var urlRx = regexp.MustCompile(`([a-z]+)://(.*)`)
var gcpTopicRx = regexp.MustCompile(`^projects/([^/]+)/topics/([^/]+)$`)
var rabbitTopicRx = regexp.MustCompile(`^(\w+:\w+@\w+:\d+)/topics/([^ /]+)$`)

func schemeSplit(s string) (scheme, nonScheme string, err error) {
	m := urlRx.FindStringSubmatch(s)
	if len(m) != 3 {
		return "", "", fmt.Errorf("URL %q failed to match pattern %q", s, urlRx)
	}
	scheme = m[1]
	nonScheme = m[2]
	return
}

func parseTopicURL(s string) (URL, error) {
	scheme, nonScheme, err := schemeSplit(s)
	if err != nil {
		return URL{}, fmt.Errorf("parsing topic url: %v", err)
	}
	switch scheme {
	case "gcppubsub":
		m := gcpTopicRx.FindStringSubmatch(nonScheme)
		if len(m) != 3 {
			return URL{}, fmt.Errorf("got %q, want match against %q", nonScheme, gcpTopicRx)
		}
		return URL{Provider: "gcp", Project: m[1], Topic: m[2]}, nil
	case "rabbitpubsub":
		m := rabbitTopicRx.FindStringSubmatch(nonScheme)
		if len(m) != 3 {
			return URL{}, fmt.Errorf("got %q, want match against %q", nonScheme, rabbitTopicRx)
		}
		return URL{Provider: "rabbit", ServerURL: "amqp://" + m[1], Topic: m[2]}, nil
	case "":
		return URL{}, fmt.Errorf(`scheme missing from URL: "%s"`, s)
	default:
		return URL{}, fmt.Errorf(`unrecognized scheme "%s" in URL "%s"`, scheme, s)
	}
}

var gcpSubscriptionRx = regexp.MustCompile(`^projects/([^/]+)/subscriptions/([^/]+)$`)
var rabbitSubscriptionRx = regexp.MustCompile(`^(\w+:\w+@\w+:\d+)/subscriptions/([^ /]+)$`)

func parseSubscriptionURL(s string) (URL, error) {
	scheme, nonScheme, err := schemeSplit(s)
	if err != nil {
		return URL{}, fmt.Errorf("parsing subscription url: %v", err)
	}
	switch scheme {
	case "gcppubsub":
		m := gcpSubscriptionRx.FindStringSubmatch(nonScheme)
		return URL{Provider: "gcp", Project: m[1], Subscription: m[2]}, nil
	case "rabbitpubsub":
		m := rabbitSubscriptionRx.FindStringSubmatch(nonScheme)
		return URL{Provider: "rabbit", ServerURL: "amqp://" + m[1], Subscription: m[2]}, nil
	case "":
		return URL{}, fmt.Errorf(`scheme missing from URL: "%s"`, s)
	default:
		return URL{}, fmt.Errorf(`unrecognized scheme "%s" in URL "%s"`, scheme, s)
	}
}

func openTopic(ctx context.Context, u URL) (*pubsub.Topic, func(), error) {
	switch u.Provider {
	case "gcp":
		return openGCPTopic(ctx, u.Project, u.Topic)
	case "rabbit":
		return openRabbitTopic(u.ServerURL, u.Topic)
	}
	return nil, nil, fmt.Errorf("unrecognized provider: %s", u.Provider)
}

func openSubscription(ctx context.Context, u URL) (*pubsub.Subscription, func(), error) {
	switch u.Provider {
	case "gcp":
		return openGCPSubscription(ctx, u.Project, u.Subscription)
	case "rabbit":
		return openRabbitSubscription(u.ServerURL, u.Subscription)
	}
	return nil, nil, fmt.Errorf("unrecognized provider: %s", u.Provider)
}
