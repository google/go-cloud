package azurepubsub

import (
	"os"
	"fmt"
	"context"
	"errors"
	"testing"

	"gocloud.dev/pubsub"

	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"

	"github.com/Azure/azure-service-bus-go"
)
var (
	connString = os.Getenv("SERVICEBUS_CONNECTION_STRING")
)
const (
	
	topicName         = "test-topic"
	subscriptionName0 = "test-subscription-1"
	subscriptionName1 = "test-subscription-2"
)

type harness struct {
	closer     func()
	connString string
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return &harness{
		connString: connString,
		closer: func() {

		},
	}, nil
}

func (h *harness) MakeTopic(ctx context.Context) (driver.Topic, error) {
	dt, err := openTopic(ctx, topicName, h.connString, nil)
	return dt, err
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	dt, err := openTopic(ctx, "nonexistent-topic", h.connString, nil)
	return dt, err
}

func (h *harness) MakeSubscription(ctx context.Context, dt driver.Topic, n int) (driver.Subscription, error) {
	var sname string
	switch n {
	case 0:
		sname = subscriptionName0
	case 1:
		sname = subscriptionName1
	default:
		return nil, errors.New("n must be 0 or 1")
	}
	return openSubscription(ctx, topicName, sname, h.connString, nil)
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	ds, err := openSubscription(ctx, topicName, "nonexistent-subscription", h.connString, nil)
	return ds, err

}

func (h *harness) Close() {
	h.closer()
}

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{sbAsTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

type sbAsTest struct{}

func (sbAsTest) Name() string {
	return "azure servicebus test"
}

func (sbAsTest) TopicCheck(top *pubsub.Topic) error {	
	var t2 servicebus.Topic
	if top.As(&t2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &t2)
	}
	var t3 *servicebus.Topic
	if !top.As(&t3) {
		return fmt.Errorf("cast failed for %T", &t3)
	}
	return nil
}

func (sbAsTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	
	var s2 servicebus.Subscription
	if sub.As(&s2) {
		return fmt.Errorf("cast succeeded for %T, want failure", &s2)
	}
	var s3 *servicebus.Subscription
	if !sub.As(&s3) {
		return fmt.Errorf("cast failed for %T", &s3)
	}
	return nil	
}
