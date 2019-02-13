package psutil_test

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/mempubsub"
	"gocloud.dev/pubsub/psutil"

	"gocloud.dev/gcerrors"
	"gocloud.dev/pubsub/driver"
)

func TestReceiveConcurrently(t *testing.T) {
	t.Run("can be cancelled", func(t *testing.T) {
		top := mempubsub.NewTopic()
		sub := mempubsub.NewSubscription(top, time.Second)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := psutil.ReceiveConcurrently(ctx, sub, 10, func(ctx context.Context, m *pubsub.Message) error {
			return nil // ack
		})
		if err == nil {
			t.Fatal("got nil, want error")
		}
		if !strings.Contains(err.Error(), "cancel") {
			t.Errorf(`psutil.ReceiveConcurrently returned error "%v", want error containing 'cancel'`, err)
		}
	})
	t.Run("returns error if receive fails", func(t *testing.T) {
		ctx := context.Background()
		wantErr := fmt.Errorf("error-%d", rand.Int())
		ds := &erroringDriverSub{err: wantErr}
		sub := pubsub.NewSubscription(ds, nil)
		err := psutil.ReceiveConcurrently(ctx, sub, 10, func(ctx context.Context, m *pubsub.Message) error {
			return nil // ack
		})
		if !strings.Contains(err.Error(), wantErr.Error()) {
			t.Errorf(`psutil.ReceiveConcurrently returned error of "%v", want it to contain "%v"`, err, wantErr)
		}
	})
	t.Run("returns error if message handler fails", func(t *testing.T) {
		ctx := context.Background()
		top := mempubsub.NewTopic()
		sub := mempubsub.NewSubscription(top, time.Second)
		wantErr := fmt.Errorf("error-%d", rand.Int())
		if err := top.Send(ctx, &pubsub.Message{}); err != nil {
			t.Fatalf("sending message to topic: %v", err)
		}
		err := psutil.ReceiveConcurrently(ctx, sub, 10, func(ctx context.Context, m *pubsub.Message) error {
			return wantErr
		})
		if !strings.Contains(err.Error(), wantErr.Error()) {
			t.Errorf(`psutil.ReceiveConcurrently returned error of "%v", want it to contain "%v"`, err, wantErr)
		}
	})
	for max := 1; max <= 1000; max *= 10 {
		t.Run(fmt.Sprintf("can process messages with max %d", max), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			top := mempubsub.NewTopic()
			sub := mempubsub.NewSubscription(top, time.Second)

			t.Logf("sending a bunch of messages to the topic and store their bodies in a set (map)")
			var mu sync.Mutex
			set := make(map[string]bool)
			N := 10 * max
			for i := 0; i < N; i++ {
				b := fmt.Sprintf("%d", i)
				set[b] = true
				m := &pubsub.Message{Body: []byte(b)}
				if err := top.Send(ctx, m); err != nil {
					t.Fatalf("sending message with body %q to topic: %v", b, top)
				}
			}
			errc := make(chan error)

			t.Logf("starting the worker pool")
			go func() {
				errc <- psutil.ReceiveConcurrently(ctx, sub, max, func(ctx context.Context, m *pubsub.Message) error {
					mu.Lock()
					delete(set, string(m.Body))
					mu.Unlock()
					return nil
				})
			}()

			t.Logf("waiting for messages to be processed")
			for {
				mu.Lock()
				// n is the number of messages remaining to be processed
				n := len(set)
				mu.Unlock()
				if n == 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}

			t.Logf("stopping the worker pool")
			cancel()
			err := <-errc
			if !strings.Contains(err.Error(), "canceled") {
				t.Errorf(`psutil.ReceiveConcurrently returned error of "%v", want it to contain "canceled"`, err)
			}
		})
	}
}

type erroringDriverSub struct {
	driver.Subscription
	// err is the error to return
	err error
}

func (s *erroringDriverSub) ReceiveBatch(ctx context.Context, maxMessages int) ([]*driver.Message, error) {
	return nil, s.err
}

func (s *erroringDriverSub) SendAcks(ctx context.Context, ackIDs []driver.AckID) error {
	return nil
}

func (s *erroringDriverSub) IsRetryable(error) bool { return false }

func (s *erroringDriverSub) ErrorCode(error) gcerrors.ErrorCode { return gcerrors.Internal }
