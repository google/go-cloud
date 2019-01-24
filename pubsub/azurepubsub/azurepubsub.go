// Copyright 2018 The Go Cloud Authors
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

// Package azurepubsub provides an implementation of pubsub that uses Azure Service Bus Topics and Subscriptions
// PubSub.
//
// It exposes the following types for As:
// Topic: *servicebus.Topic
// Subscription: *servicebus.Subscription
package azurepubsub // import "gocloud.dev/pubsub/azurepubsub"

import (
	"context"
	"fmt"	
	"pack.ag/amqp"
	"runtime"
	"sync"
	"time"
	
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"

	"github.com/Azure/azure-amqp-common-go/cbs"
	"github.com/Azure/azure-amqp-common-go/rpc"
	"github.com/Azure/azure-amqp-common-go/uuid"
	"github.com/Azure/azure-service-bus-go"
)

const (
	rootUserAgent = "/golang-service-bus"
	completedStatus  = "completed"	
	listenerTimeout = 1 * time.Second
	rpcTries = 5
	rpcRetryDelay = 1 * time.Second
)

type topic struct {		
	name string
	ns *servicebus.Namespace	
}

// TopicOptions provides configuration options for an Azure SB Topic.
type TopicOptions struct {}

// OpenTopic opens the topic on Azure ServiceBus PubSub for the given topicName.
func OpenTopic(ctx context.Context, topicName string, connString string, opts *TopicOptions) (*pubsub.Topic) {
	t, err := openTopic(ctx, topicName, connString, opts)
	if err != nil {
		return nil
	}
	return pubsub.NewTopic(t)
}

// openTopic returns the driver for OpenTopic. This function exists so the test
// harness can get the driver interface implementation if it needs to.
func openTopic(ctx context.Context, topicName, connectionString string, opts *TopicOptions) (driver.Topic, error) {		
	ns, err := getSbNamespace(connectionString)
	if err != nil {
		return nil, err
	}

	t := &topic {
		name: topicName,		
		ns : ns,		
	}
	return t, nil
}

func createTopic(ctx context.Context, topicName, connectionString string, opts[] servicebus.TopicManagementOption) (error) {		
	ns, err := getSbNamespace(connectionString)
	if err != nil{
		return err
	}	
	tm := ns.NewTopicManager()	
	_, err = ensureTopic(ctx, topicName, tm, opts...)
	return err
}

func ensureTopic(ctx context.Context, topicName string, topicManager *servicebus.TopicManager, opts ...servicebus.TopicManagementOption) (*servicebus.TopicEntity, error) {
	te, err := topicManager.Get(ctx, topicName)
	
	if err == nil {
		_ = topicManager.Delete(ctx, topicName)
	}

	te, err = topicManager.Put(ctx, topicName, opts...)
	if err != nil {		
		return nil, err
	}
	return te, nil	
}

func deleteTopic(ctx context.Context, topicName, connectionString string) (error) {	
	ns, err := getSbNamespace(connectionString)
	if err != nil{
		return err
	}	
	tm := ns.NewTopicManager()	
	te, err := tm.Get(ctx, topicName)
	if te != nil {
	 return tm.Delete(ctx, topicName)	
	}
	return nil
}	

func getSbNamespace(connectionString string)(*servicebus.Namespace, error){
	// Ensure the Azure ServcieBus ConnectionString is correctly formatted.
	// See https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-sas
	nsOptions := servicebus.NamespaceWithConnectionString(connectionString)
	return servicebus.NewNamespace(nsOptions)
}

func (t *topic) getSbTopic() (*servicebus.Topic, error){
	return t.ns.NewTopic(t.name)
}

// SendBatch implements driver.Topic.SendBatch.
func (t *topic) SendBatch(ctx context.Context, dms []*driver.Message) (error){		
	sbTopic, err := t.getSbTopic()
	if err != nil {
		return nil
	}
	defer func(){
		sbTopic.Close(ctx)
	}()

	topicSender, err := sbTopic.NewSender(ctx)
	if err != nil {
		return err
	}
	defer func(){
		topicSender.Close(ctx)
	}()
	
	var sendError error
	for i, dm := range dms {
		sbms :=servicebus.NewMessage(dm.Body)				
		for k, v := range dm.Metadata {
			sbms.Set(k, v)
		}		

		sendError := topicSender.Send(ctx, sbms)
		if sendError != nil {
			sendError = fmt.Errorf("azure pubsub failed to send at %v: %v", i, sendError)
			break
		}
	}	
	return sendError
}

func (t *topic) IsRetryable(err error) (bool){
	return false
}

func (t *topic) As(i interface{}) bool{
	p, ok := i.(**servicebus.Topic)
	if !ok {
		return false
	}	
	*p , _= t.getSbTopic()
	return true
}

type subscription struct {
	name string
	topicName string
	ns *servicebus.Namespace
}

// SubscriptionOptions will contain configuration for subscriptions.
type SubscriptionOptions struct{}

// OpenSubscription opens a ServiceBus Subscription on the given topicName and namespace connection string.
func OpenSubscription(ctx context.Context, topicName string, subscriptionName string, connString string, opts *SubscriptionOptions) (*pubsub.Subscription, error) {
	s, err := openSubscription(ctx, topicName, subscriptionName, connString, opts)
	if err != nil {
		return nil, err
	}

	return pubsub.NewSubscription(s, nil), nil
}

// openSubscription returns a driver.Subscription.
func openSubscription(ctx context.Context, topicName string, subscriptionName string, connectionString string, opts *SubscriptionOptions) (driver.Subscription, error) {
	ns, err := getSbNamespace(connectionString)
	if err != nil{
		return nil, err
	}
	return &subscription {		
		name : subscriptionName,
		topicName: topicName,		
		ns: ns,
	}, nil
}

func createSubscription(ctx context.Context, topicName string, subscriptionName string, connectionString string, opts[] servicebus.SubscriptionManagementOption) (error) {
	ns, err := getSbNamespace(connectionString)
	if err != nil {
		return err
	}

	subManager, err := ns.NewSubscriptionManager(topicName)
	if err != nil {
		return err
	}
	
	_, err = ensureSubscription(ctx, subManager, subscriptionName, opts...)	
	return err	
}

func ensureSubscription(ctx context.Context, sm *servicebus.SubscriptionManager, name string, opts ...servicebus.SubscriptionManagementOption) (*servicebus.SubscriptionEntity, error) {
	subEntity, err := sm.Get(ctx, name)
	if err == nil {
		_ = sm.Delete(ctx, name)
	}

	subEntity, err = sm.Put(ctx, name, opts...)
	if err != nil {		
		return nil, err
	}

	return subEntity, nil
}

func deleteSubscription(ctx context.Context, topicName string, subscriptionName string, connectionString string) (error){
	ns, err := getSbNamespace(connectionString)
	if err != nil {
		return err
	}
	sm, err := ns.NewSubscriptionManager(topicName)
	se, err := sm.Get(ctx, subscriptionName)
	if se != nil {
		_ = sm.Delete(ctx, subscriptionName)
	}
	return nil
}

func (s *subscription) getSbSubscription(ctx context.Context)(*servicebus.Subscription, error){
	sm, err := s.ns.NewSubscriptionManager(s.topicName)	
	if err != nil {
		return nil, err
	}
	// An empty SubscriptionEntity means no subscription exist for the given name. 
	se, err := sm.Get(ctx, s.name)
	if se == nil {
		return nil, fmt.Errorf("azurepubsub: no such subscription %v", s.name)
	}

	sbTopic, err := s.ns.NewTopic(s.topicName)	
	if err != nil {
		return nil, err
	}
	return sbTopic.NewSubscription(s.name)
}

// IsRetryable implements driver.Subscription.IsRetryable.
func (s *subscription) IsRetryable(error) bool {	
	return false
}

// As implements driver.Subscription.As.
func (s *subscription) As(i interface{}) bool {
	p, ok := i.(**servicebus.Subscription)
	if !ok {
		return false
	}	
	*p, _ = s.getSbSubscription(context.Background())
	return true
}

// ReceiveBatch implements driver.Subscription.ReceiveBatch.
func (s *subscription) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {			
	sbSub, err := s.getSbSubscription(ctx)
	if err != nil {
		return nil, err
	}
	defer func(){
		sbSub.Close(ctx)
	}()
	
	receiverCtx, cancelCtx := context.WithTimeout(ctx, listenerTimeout)
	defer cancelCtx()
	
	subReceiver, err := sbSub.NewReceiver(ctx)
	if err != nil {
		return nil, err
	}
	defer func(){
		subReceiver.Close(ctx)
	}()
	
	var messages []*driver.Message			
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		subReceiver.Listen(receiverCtx, servicebus.HandlerFunc(func(iCtx context.Context, sbmsg *servicebus.Message)(error){		
			metadata := map[string]string{}
			for k, v := range sbmsg.UserProperties {			
				metadata[k] = v.(string)
			}			
			messages = append(messages, &driver.Message{
				Body:     sbmsg.Data,
				Metadata: metadata,
				AckID:    sbmsg.LockToken,
			})			
			if len(messages) >= maxMessages {
				cancelCtx()
			}
			return nil
		}))
		
		select {
			case <- receiverCtx.Done():
				wg.Done()
		}
	}() 

	wg.Wait()		
	return messages, nil
}

// SendAcks implements driver.Subscription.SendAcks.
// IMPORTANT: This is a workaround to issue 'completed' message dispositions in bulk which is not supported in the ServiceBus SDK.  
func (s *subscription) SendAcks(ctx context.Context, ids []driver.AckID) error {
	if len (ids) == 0 {
		return nil
	}
	
	host := fmt.Sprintf("amqps://%s.%s/", s.ns.Name, s.ns.Environment.ServiceBusEndpointSuffix)
	client, err := amqp.Dial(host,
		amqp.ConnSASLAnonymous(),
		amqp.ConnProperty("product", "Go-Cloud Client"),
		amqp.ConnProperty("version", servicebus.Version),
		amqp.ConnProperty("platform", runtime.GOOS),
		amqp.ConnProperty("framework", runtime.Version()),
		amqp.ConnProperty("user-agent", rootUserAgent),
	)
	if err != nil {
		return err
	}
		
	entityPath := s.topicName+"/Subscriptions/"+s.name		
	audience := host + entityPath
	err = cbs.NegotiateClaim(ctx, audience, client, s.ns.TokenProvider)
	if err != nil {
		return nil
	}
	
	lockIds := []amqp.UUID{}
	for _, mid := range ids {
		if id, ok := mid.(*uuid.UUID); ok {			
			lockTokenBytes := [16]byte(*id)
			lockIds = append(lockIds, amqp.UUID(lockTokenBytes))
		}
	}

	value := map[string]interface{}{
		"disposition-status": completedStatus,
		"lock-tokens":        lockIds,
	}

	msg := &amqp.Message{
		ApplicationProperties: map[string]interface{}{
			"operation": "com.microsoft:update-disposition",
		},
		Value: value,
	}
	
	sbSub, err := s.getSbSubscription(ctx)
	if err != nil {		
		return err
	}
	link, err := rpc.NewLink(client, sbSub.ManagementPath())
	if err != nil {		
		return err
	}	
	_, err = link.RetryableRPC(ctx, rpcTries, rpcRetryDelay, msg)
	return err	
}