package azurepubsub

import (

	"os"
	"strings"
	"sync/atomic"
	"fmt"
	"context"	
	"testing"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/drivertest"
	"gocloud.dev/internal/testing/setup"

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
	numTopics uint32 // atomic
	numSubs   uint32 // atomic
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	return &harness{
		connString: connString,
		closer: func() {

		},
	}, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (dt driver.Topic, cleanup func(), err error) {
	topicName := fmt.Sprintf("%s-topic-%d", sanitize(testName), atomic.AddUint32(&h.numTopics, 1))
	
	err = createTopic(ctx, topicName, h.connString, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating topic: %v", err)
	}
	dt, err = openTopic(ctx, topicName, h.connString, nil)
	cleanup = func() {
		deleteTopic(ctx, topicName, h.connString )
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	dt, err := openTopic(ctx, "nonexistent-topic", h.connString, nil)
	return dt, err
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (ds driver.Subscription, cleanup func(), err error) {
	// Azure subscription name has to be less than 50 characters.
	subName := fmt.Sprintf("%s-sub-%d", sanitize(testName), atomic.AddUint32(&h.numSubs, 1))	
	t := dt.(*topic)

	err = createSubscription(ctx, t.name, subName, h.connString, nil)
	if err != nil {
		return nil, nil, err
	}

	ds, err = openSubscription(ctx, t.name, subName, h.connString, nil)

	cleanup = func() {
		deleteSubscription(ctx, t.name, subName, h.connString)
	}

	return ds, cleanup, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	ds, err := openSubscription(ctx, topicName, "nonexistent-subscription", h.connString, nil)
	return ds, err
}

func (h *harness) Close() {
	h.closer()
}

// Please run the TestConformance with an extended timeout since each test needs to preform CRUD for ServiceBus Topics and Subscriptions.
// Example: C:\Go\bin\go.exe test -timeout 60s gocloud.dev/pubsub/azurepubsub -run ^TestConformance$
func TestConformance(t *testing.T) {
	if !*setup.Record {
        t.Skip("replaying is not yet supported for Azure pubsub")
    
	} else {
		asTests := []drivertest.AsTest{sbAsTest{}}
		drivertest.RunConformanceTests(t, newHarness, asTests)
	}
}

type sbAsTest struct{}

func (sbAsTest) Name() string {
	return "sb"
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

func sanitize(testName string) string {
	// Keep the entity name under 50 characters as per Azure limits.
	// See https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-quotas
	name := strings.Replace(testName, "/", "_", -1)
	if len(name) > 40 {
		name = name[:len(name)-20]
	}
	return name
}