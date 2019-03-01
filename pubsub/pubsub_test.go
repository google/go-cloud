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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/testing/octest"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/driver"
	"gocloud.dev/pubsub/mempubsub"
	"golang.org/x/sync/errgroup"
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

func (s *driverTopic) IsRetryable(error) bool { return false }

func (s *driverTopic) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }

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

func (s *driverSub) IsRetryable(error) bool { return false }

func (*driverSub) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }

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

	sub := pubsub.NewSubscription(ds, nil)
	defer sub.Shutdown(ctx)
	m2, err := sub.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(m2.Body) != string(m.Body) {
		t.Fatalf("received message has body %q, want %q", m2.Body, m.Body)
	}
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
	s := pubsub.NewSubscription(ds, nil)
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
	s := pubsub.NewSubscription(ds, nil)
	defer s.Shutdown(ctx)
	cancel()
	// Without cancellation, this Receive would hang.
	if _, err := s.Receive(ctx); err == nil {
		t.Error("got nil, want cancellation error")
	}
}

type blockingDriverSub struct {
	driver.Subscription
	inReceiveBatch chan int
}

func (b blockingDriverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	b.inReceiveBatch <- 0
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCancelTwoReceives(t *testing.T) {
	// We want to create the following situation:
	// 1. Goroutine 1 calls Receive, obtains the lock (Subscription.mu),
	//    and calls driver.ReceiveBatch, which hangs forever.
	// 2. Goroutine 2 calls Receive.
	// 3. The context passed to the Goroutine 2 call is canceled.
	// We expect Goroutine 2's Receive to exit immediately. That won't
	// happen if Receive holds the lock during the call to ReceiveBatch.
	inReceiveBatch := make(chan int, 1)
	s := pubsub.NewSubscription(blockingDriverSub{inReceiveBatch: inReceiveBatch}, nil)
	go func() {
		s.Receive(context.Background())
		t.Fatal("Receive should never return")
	}()
	<-inReceiveBatch
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)
	go func() {
		_, err := s.Receive(ctx)
		errc <- err
	}()
	// Give the Receive call time to block on the mutex before canceling.
	time.AfterFunc(100*time.Millisecond, cancel)
	err := <-errc
	if err != context.Canceled {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestRetryTopic(t *testing.T) {
	// Test that Send is retried if the driver returns a retryable error.
	ft := &failTopic{}
	top := pubsub.NewTopic(ft, nil)
	err := top.Send(context.Background(), &pubsub.Message{})
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

func (t *failTopic) IsRetryable(err error) bool { return isRetryable(err) }

func (*failTopic) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Unknown }

func TestRetryReceive(t *testing.T) {
	fs := &failSub{}
	sub := pubsub.NewSubscription(fs, nil)
	_, err := sub.Receive(context.Background())
	if err != nil {
		t.Errorf("Receive: got %v, want nil", err)
	}
	if got, want := fs.calls, nRetryCalls+1; got != want {
		t.Errorf("calls: got %d, want %d", got, want)
	}
}

type failSub struct {
	driver.Subscription
	calls int
}

func (t *failSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	t.calls++
	if t.calls <= nRetryCalls {
		return nil, errRetry
	}
	return []*driver.Message{{Body: []byte("")}}, nil
}

func (t *failSub) IsRetryable(err error) bool { return isRetryable(err) }

// TODO(jba): add a test for retry of SendAcks.

var errDriver = errors.New("driver error")

type erroringTopic struct {
	driver.Topic
}

func (erroringTopic) SendBatch(context.Context, []*driver.Message) error { return errDriver }
func (erroringTopic) IsRetryable(err error) bool                         { return isRetryable(err) }
func (erroringTopic) ErrorCode(error) gcerrors.ErrorCode                 { return gcerrors.AlreadyExists }

type erroringSubscription struct {
	driver.Subscription
}

func (erroringSubscription) ReceiveBatch(context.Context, int) ([]*driver.Message, error) {
	return nil, errDriver
}

func (erroringSubscription) SendAcks(context.Context, []driver.AckID) error { return errDriver }
func (erroringSubscription) IsRetryable(err error) bool                     { return isRetryable(err) }
func (erroringSubscription) ErrorCode(error) gcerrors.ErrorCode             { return gcerrors.AlreadyExists }

// TestErrorsAreWrapped tests that all errors returned from the driver are
// wrapped exactly once by the concrete type.
func TestErrorsAreWrapped(t *testing.T) {
	ctx := context.Background()
	top := pubsub.NewTopic(erroringTopic{}, nil)
	sub := pubsub.NewSubscription(erroringSubscription{}, nil)

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

	verify(top.Send(ctx, &pubsub.Message{}))
	_, err := sub.Receive(ctx)
	verify(err)
}

func TestOpenCensus(t *testing.T) {
	ctx := context.Background()
	te := octest.NewTestExporter(pubsub.OpenCensusViews)
	defer te.Unregister()

	top := mempubsub.NewTopic()
	sub := mempubsub.NewSubscription(top, time.Second)
	if err := top.Send(ctx, &pubsub.Message{Body: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if err := top.Shutdown(ctx); err != nil {
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
		{"driver.Topic.SendBatch", gcerrors.OK},
		{"Topic.Send", gcerrors.OK},
		{"Topic.Shutdown", gcerrors.OK},
		{"driver.Subscription.ReceiveBatch", gcerrors.OK},
		{"Subscription.Receive", gcerrors.OK},
		{"driver.Subscription.SendAcks", gcerrors.OK},
		{"Subscription.Shutdown", gcerrors.OK},
		{"Subscription.Receive", gcerrors.FailedPrecondition},
	})
	if diff != "" {
		t.Error(diff)
	}
}

func TestShutdownsDoNotLeakGoroutines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ng0 := runtime.NumGoroutine()
	top := mempubsub.NewTopic()
	sub := mempubsub.NewSubscription(top, time.Second)

	// Send a bunch of messages at roughly the same time to make the batcher's work more difficult.
	var eg errgroup.Group
	n := 1000
	for i := 0; i < n; i++ {
		eg.Go(func() error {
			return top.Send(ctx, &pubsub.Message{})
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// Receive the messages.
	var eg2 errgroup.Group
	for i := 0; i < n; i++ {
		eg2.Go(func() error {
			m, err := sub.Receive(ctx)
			if err != nil {
				return err
			}
			m.Ack()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// The Shutdown methods each spawn a goroutine so we want to make sure those don't
	// keep running indefinitely after the Shutdowns return.
	cancel()
	top.Shutdown(ctx)
	sub.Shutdown(ctx)

	// Wait for number of goroutines to return to normal.
	// Otherwise the test hangs.
	for {
		ng := runtime.NumGoroutine()
		if ng == ng0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
}

var (
	testOpenOnce sync.Once
	testOpenGot  *url.URL
)

func TestURLMux(t *testing.T) {
	ctx := context.Background()
	var gotTopic, gotSub *url.URL

	mux := new(pubsub.URLMux)
	// Register scheme foo to always return nil. Sets got as a side effect
	mux.RegisterTopic("foo", topicURLOpenFunc(func(_ context.Context, u *url.URL) (*pubsub.Topic, error) {
		gotTopic = u
		return nil, nil
	}))
	mux.RegisterSubscription("foo", subscriptionURLOpenFunc(func(_ context.Context, u *url.URL) (*pubsub.Subscription, error) {
		gotSub = u
		return nil, nil
	}))
	// Register scheme err to always return an error.
	mux.RegisterTopic("err", topicURLOpenFunc(func(_ context.Context, u *url.URL) (*pubsub.Topic, error) {
		return nil, errors.New("fail")
	}))
	mux.RegisterSubscription("err", subscriptionURLOpenFunc(func(_ context.Context, u *url.URL) (*pubsub.Subscription, error) {
		return nil, errors.New("fail")
	}))

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
	} {
		t.Run("topic: "+tc.name, func(t *testing.T) {
			_, gotErr := mux.OpenTopic(ctx, tc.url)
			if (gotErr != nil) != tc.wantErr {
				t.Fatalf("got err %v, want error %v", gotErr, tc.wantErr)
			}
			if gotErr != nil {
				return
			}
			want, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(gotTopic, want); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", gotTopic, want, diff)
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
			want, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(gotSub, want); diff != "" {
				t.Errorf("got\n%v\nwant\n%v\ndiff\n%s", gotSub, want, diff)
			}
		})
	}
}

type topicURLOpenFunc func(context.Context, *url.URL) (*pubsub.Topic, error)

func (f topicURLOpenFunc) OpenTopicURL(ctx context.Context, u *url.URL) (*pubsub.Topic, error) {
	return f(ctx, u)
}

type subscriptionURLOpenFunc func(context.Context, *url.URL) (*pubsub.Subscription, error)

func (f subscriptionURLOpenFunc) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*pubsub.Subscription, error) {
	return f(ctx, u)
}
