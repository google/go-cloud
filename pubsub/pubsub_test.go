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
package pubsub_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/testing/octest"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/mempubsub"
)

type driverTopic struct {
	driver.Topic
	subs []*driverSub
}

func (t *driverTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	for _, s := range t.subs {
		select {
		case <-s.sem:
			s.q = append(s.q, ms...)
			s.sem <- struct{}{}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (*driverTopic) IsRetryable(error) bool             { return false }
func (*driverTopic) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }
func (*driverTopic) Close() error                       { return nil }

type driverSub struct {
	driver.Subscription
	sem chan struct{}
	// Normally this queue would live on a separate server in the cloud.
	q []*driver.Message
}

func NewDriverSub() *driverSub {
	ds := &driverSub{
		sem: make(chan struct{}, 1),
	}
	ds.sem <- struct{}{}
	return ds
}

func (s *driverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	for {
		select {
		case <-s.sem:
			ms := s.grabQueue(maxMessages)
			if len(ms) != 0 {
				return ms, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
}

func (s *driverSub) grabQueue(maxMessages int) []*driver.Message {
	defer func() { s.sem <- struct{}{} }()
	if len(s.q) > 0 {
		if len(s.q) <= maxMessages {
			ms := s.q
			s.q = nil
			return ms
		}
		ms := s.q[:maxMessages]
		s.q = s.q[maxMessages:]
		return ms
	}
	return nil
}

func (s *driverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return nil
}

func (*driverSub) IsRetryable(error) bool             { return false }
func (*driverSub) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }
func (*driverSub) CanNack() bool                      { return false }
func (*driverSub) Close() error                       { return nil }

func TestSendReceive(t *testing.T) {
	ctx := context.Background()
	ds := NewDriverSub()
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)
	m := &pubsub.Message{Body: []byte("user signed up")}
	if err := topic.Send(ctx, m); err != nil {
		t.Fatal(err)
	}

	sub := pubsub.NewSubscription(ds, nil, nil)
	defer sub.Shutdown(ctx)
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(m2.Body) != string(m.Body) {
		t.Fatalf("received message has body %q, want %q", m2.Body, m.Body)
	}
	m2.Ack()
}

func TestConcurrentReceivesGetAllTheMessages(t *testing.T) {
	howManyToSend := int(1e3)
	ctx, cancel := context.WithCancel(context.Background())
	dt := &driverTopic{}

	// wg is used to wait until all messages are received.
	var wg sync.WaitGroup
	wg.Add(howManyToSend)

	// Make a subscription.
	ds := NewDriverSub()
	dt.subs = append(dt.subs, ds)
	s := pubsub.NewSubscription(ds, nil, nil)
	defer s.Shutdown(ctx)

	// Start 10 goroutines to receive from it.
	var mu sync.Mutex
	receivedMsgs := make(map[string]bool)
	for i := 0; i < 10; i++ {
		go func() {
			for {
				m, err := s.Receive(ctx)
				if err != nil {
					// Permanent error; ctx cancelled or subscription closed is
					// expected once we've received all the messages.
					mu.Lock()
					n := len(receivedMsgs)
					mu.Unlock()
					if n != howManyToSend {
						t.Errorf("Worker's Receive failed before all messages were received (%d)", n)
					}
					return
				}
				m.Ack()
				mu.Lock()
				receivedMsgs[string(m.Body)] = true
				mu.Unlock()
				wg.Done()
			}
		}()
	}

	// Send messages. Each message has a unique body used as a key to receivedMsgs.
	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)
	for i := 0; i < howManyToSend; i++ {
		key := fmt.Sprintf("message #%d", i)
		m := &pubsub.Message{Body: []byte(key)}
		if err := topic.Send(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for the goroutines to receive all of the messages, then cancel the
	// ctx so they all exit.
	wg.Wait()
	defer cancel()

	// Check that all the messages were received.
	for i := 0; i < howManyToSend; i++ {
		key := fmt.Sprintf("message #%d", i)
		if !receivedMsgs[key] {
			t.Errorf("message %q was not received", key)
		}
	}
}

func TestCancelSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ds := NewDriverSub()
	dt := &driverTopic{
		subs: []*driverSub{ds},
	}
	topic := pubsub.NewTopic(dt, nil)
	defer topic.Shutdown(ctx)
	m := &pubsub.Message{}

	// Intentionally break the driver subscription by acquiring its semaphore.
	// Now topic.Send will have to wait for cancellation.
	<-ds.sem

	cancel()
	if err := topic.Send(ctx, m); err == nil {
		t.Error("got nil, want cancellation error")
	}
}

func TestCancelReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ds := NewDriverSub()
	s := pubsub.NewSubscription(ds, nil, nil)
	defer s.Shutdown(ctx)
	cancel()
	// Without cancellation, this Receive would hang.
	if _, err := s.Receive(ctx); err == nil {
		t.Error("got nil, want cancellation error")
	}
}

type blockingDriverSub struct {
	driver.Subscription
	inReceiveBatch chan struct{}
}

func (b blockingDriverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	b.inReceiveBatch <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingDriverSub) CanNack() bool          { return false }
func (blockingDriverSub) IsRetryable(error) bool { return false }
func (blockingDriverSub) Close() error           { return nil }

func TestCancelTwoReceives(t *testing.T) {
	// We want to create the following situation:
	// 1. Goroutine 1 calls Receive, obtains the lock (Subscription.mu),
	//    then releases the lock and calls driver.ReceiveBatch, which hangs.
	// 2. Goroutine 2 calls Receive.
	// 3. The context passed to the Goroutine 2 call is canceled.
	// We expect Goroutine 2's Receive to exit immediately. That won't
	// happen if Receive holds the lock during the call to ReceiveBatch.
	inReceiveBatch := make(chan struct{})
	s := pubsub.NewSubscription(blockingDriverSub{inReceiveBatch: inReceiveBatch}, nil, nil)
	defer s.Shutdown(context.Background())
	go func() {
		_, err := s.Receive(context.Background())
		// This should happen at the very end of the test, during Shutdown.
		if err != context.Canceled {
			t.Errorf("got %v, want context.Canceled", err)
		}
	}()
	<-inReceiveBatch
	// Give the Receive call time to block on the mutex before timing out.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	errc := make(chan error)
	go func() {
		_, err := s.Receive(ctx)
		errc <- err
	}()
	err := <-errc
	if err != context.DeadlineExceeded {
		t.Errorf("got %v, want context.DeadlineExceeded", err)
	}
}

func TestRetryTopic(t *testing.T) {
	// Test that Send is retried if the driver returns a retryable error.
	ctx := context.Background()
	ft := &failTopic{}
	topic := pubsub.NewTopic(ft, nil)
	defer topic.Shutdown(ctx)
	err := topic.Send(ctx, &pubsub.Message{})
	if err != nil {
		t.Errorf("Send: got %v, want nil", err)
	}
	if got, want := ft.calls, nRetryCalls+1; got != want {
		t.Errorf("calls: got %d, want %d", got, want)
	}
}

var errRetry = errors.New("retry")

func isRetryable(err error) bool {
	return err == errRetry
}

const nRetryCalls = 2

// failTopic helps test retries for SendBatch.
//
// SendBatch will fail nRetryCall times before succeeding.
type failTopic struct {
	driver.Topic
	calls int
}

func (t *failTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	t.calls++
	if t.calls <= nRetryCalls {
		return errRetry
	}
	return nil
}

func (*failTopic) IsRetryable(err error) bool         { return isRetryable(err) }
func (*failTopic) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }
func (*failTopic) Close() error                       { return nil }

func TestRetryReceive(t *testing.T) {
	ctx := context.Background()
	fs := &failSub{start: true}
	sub := pubsub.NewSubscription(fs, nil, nil)
	defer sub.Shutdown(ctx)
	m, err := sub.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive: got %v, want nil", err)
	}
	m.Ack()
	if got, want := fs.calls, nRetryCalls+1; got != want {
		t.Errorf("calls: got %d, want %d", got, want)
	}
}

// TestRetryReceiveBatches verifies that batching and retries work without races
// (see https://github.com/google/go-cloud/issues/2676).
func TestRetryReceiveInBatchesDoesntRace(t *testing.T) {
	ctx := context.Background()
	fs := &failSub{}
	// Allow multiple handlers and cap max batch size to ensure we get concurrency.
	sub := pubsub.NewSubscription(fs, &batcher.Options{MaxHandlers: 10, MaxBatchSize: 2}, nil)
	defer sub.Shutdown(ctx)

	// Do some receives to allow the number of batches to increase past 1.
	for n := 0; n < 100; n++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive: got %v, want nil", err)
		}
		m.Ack()
	}
	// Tell the failSub to start failing.
	fs.mu.Lock()
	fs.start = true
	fs.mu.Unlock()

	// This call to Receive should result in nRetryCalls+1 calls to ReceiveBatch for
	// each batch. In the issue noted above, this would cause a race.
	for n := 0; n < 100; n++ {
		m, err := sub.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive: got %v, want nil", err)
		}
		m.Ack()
	}
	// Don't try to verify the exact number of calls, as it is unpredictable
	// based on the timing of the batching.
}

// failSub helps test retries for ReceiveBatch.
//
// Once start=true, ReceiveBatch will fail nRetryCalls times before succeeding.
type failSub struct {
	driver.Subscription
	start bool
	calls int
	mu    sync.Mutex
}

func (t *failSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.start {
		t.calls++
		if t.calls <= nRetryCalls {
			return nil, errRetry
		}
	}
	return []*driver.Message{{Body: []byte("")}}, nil
}

func (*failSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error { return nil }
func (*failSub) IsRetryable(err error) bool                                { return isRetryable(err) }
func (*failSub) CanNack() bool                                             { return false }
func (*failSub) Close() error                                              { return nil }

// TODO(jba): add a test for retry of SendAcks.

var errDriver = errors.New("driver error")

type erroringTopic struct {
	driver.Topic
}

func (erroringTopic) SendBatch(context.Context, []*driver.Message) error { return errDriver }
func (erroringTopic) IsRetryable(err error) bool                         { return isRetryable(err) }
func (erroringTopic) ErrorCode(error) gcerrors.ErrorCode                 { return gcerrors.AlreadyExists }
func (erroringTopic) Close() error                                       { return errDriver }

type erroringSubscription struct {
	driver.Subscription
}

func (erroringSubscription) ReceiveBatch(context.Context, int) ([]*driver.Message, error) {
	return nil, errDriver
}

func (erroringSubscription) SendAcks(context.Context, []driver.AckID) error { return errDriver }
func (erroringSubscription) IsRetryable(err error) bool                     { return isRetryable(err) }
func (erroringSubscription) ErrorCode(error) gcerrors.ErrorCode             { return gcerrors.AlreadyExists }
func (erroringSubscription) CanNack() bool                                  { return false }
func (erroringSubscription) Close() error                                   { return errDriver }

// TestErrorsAreWrapped tests that all errors returned from the driver are
// wrapped exactly once by the portable type.
func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()

	verify := func(err error) {
		t.Helper()
		if err == nil {
			t.Errorf("got nil error, wanted non-nil")
			return
		}
		if e, ok := err.(*gcerr.Error); !ok {
			t.Errorf("not wrapped: %v", err)
		} else if got := e.Unwrap(); got != errDriver {
			t.Errorf("got %v for wrapped error, not errDriver", got)
		}
		if s := err.Error(); !strings.HasPrefix(s, "pubsub ") {
			t.Errorf("Error() for wrapped error doesn't start with 'pubsub': prefix: %s", s)
		}
	}

	topic := pubsub.NewTopic(erroringTopic{}, nil)
	verify(topic.Send(ctx, &pubsub.Message{}))
	err := topic.Shutdown(ctx)
	verify(err)

	sub := pubsub.NewSubscription(erroringSubscription{}, nil, nil)
	_, err = sub.Receive(ctx)
	verify(err)
	err = sub.Shutdown(ctx)
	verify(err)
}

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(pubsub.OpenCensusViews)
	defer te.Unregister()

	topic := mempubsub.NewTopic()
	defer topic.Shutdown(ctx)
	sub := mempubsub.NewSubscription(topic, time.Second)
	defer sub.Shutdown(ctx)
	if err := topic.Send(ctx, &pubsub.Message{Body: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if err := topic.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	msg.Ack()
	if err := sub.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	_, _ = sub.Receive(ctx)

	diff := octest.Diff(te.Spans(), te.Counts(), "gocloud.dev/pubsub", "gocloud.dev/pubsub/mempubsub", []octest.Call{
		{Method: "driver.Topic.SendBatch", Code: gcerrors.OK},
		{Method: "Topic.Send", Code: gcerrors.OK},
		{Method: "Topic.Shutdown", Code: gcerrors.OK},
		{Method: "driver.Subscription.ReceiveBatch", Code: gcerrors.OK},
		{Method: "Subscription.Receive", Code: gcerrors.OK},
		{Method: "driver.Subscription.SendAcks", Code: gcerrors.OK},
		{Method: "Subscription.Shutdown", Code: gcerrors.OK},
		{Method: "Subscription.Receive", Code: gcerrors.FailedPrecondition},
	})
	if diff != "" {
		t.Error(diff)
	}
}

var (
	testOpenOnce sync.Once
	testOpenGot  *url.URL
)

func TestURLMux(t *testing.T) {
	ctx := context.Background()

	mux := new(pubsub.URLMux)
	fake := &fakeOpener{}
	mux.RegisterTopic("foo", fake)
	mux.RegisterTopic("err", fake)
	mux.RegisterSubscription("foo", fake)
	mux.RegisterSubscription("err", fake)

	if diff := cmp.Diff(mux.TopicSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidTopicScheme("foo") || !mux.ValidTopicScheme("err") {
		t.Errorf("ValidTopicScheme didn't return true for valid scheme")
	}
	if mux.ValidTopicScheme("foo2") || mux.ValidTopicScheme("http") {
		t.Errorf("ValidTopicScheme didn't return false for invalid scheme")
	}

	if diff := cmp.Diff(mux.SubscriptionSchemes(), []string{"err", "foo"}); diff != "" {
		t.Errorf("Schemes: %s", diff)
	}
	if !mux.ValidSubscriptionScheme("foo") || !mux.ValidSubscriptionScheme("err") {
		t.Errorf("ValidSubscriptionScheme didn't return true for valid scheme")
	}
	if mux.ValidSubscriptionScheme("foo2") || mux.ValidSubscriptionScheme("http") {
		t.Errorf("ValidSubscriptionScheme didn't return false for invalid scheme")
	}

	for _, tc := range []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "empty URL",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     ":foo",
			wantErr: true,
		},
		{
			name:    "invalid URL no scheme",
			url:     "foo",
			wantErr: true,
		},
		{
			name:    "unregistered scheme",
			url:     "bar://myps",
			wantErr: true,
		},
		{
			name:    "func returns error",
			url:     "err://myps",
			wantErr: true,
		},
		{
			name: "no query options",
			url:  "foo://myps",
		},
		{
			name: "empty query options",
			url:  "foo://myps?",
		},
		{
			name: "query options",
			url:  "foo://myps?aAa=bBb&cCc=dDd",
		},
		{
			name: "multiple query options",
			url:  "foo://myps?x=a&x=b&x=c",
		},
		{
			name: "fancy ps name",
			url:  "foo:///foo/bar/baz",
		},
		{
			name: "using api schema prefix",
			url:  "pubsub+foo://foo",
		},
	} {
		t.Run("topic: "+tc.name, func(t *testing.T) {
			_, gotErr := mux.OpenTopic(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenTopicURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr = mux.OpenTopicURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v, want nil", gotErr)
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
		t.Run("subscription: "+tc.name, func(t *testing.T) {
			_, gotErr := mux.OpenSubscription(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
			// Repeat with OpenSubscriptionURL.
			parsed, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, gotErr = mux.OpenSubscriptionURL(ctx, parsed)
			if gotErr != nil {
				t.Fatalf("got err %v, want nil", gotErr)
			}
			if got := fake.u.String(); got != tc.url {
				t.Errorf("got %q want %q", got, tc.url)
			}
		})
	}
}

type fakeOpener struct {
	u *url.URL // last url passed to OpenTopicURL/OpenSubscriptionURL
}

func (o *fakeOpener) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return nil, nil
}

func (o *fakeOpener) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	if u.Scheme == "err" {
		return nil, errors.New("fail")
	}
	o.u = u
	return nil, nil
}
