package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cloud/internal/pubsub"
	"github.com/google/go-cloud/internal/pubsub/driver"
)

type funcTopic struct {
	sendBatch func(ctx context.Context, ms []*driver.Message) error
}

func (t *funcTopic) SendBatch(ctx context.Context, ms []*driver.Message) error {
	return t.sendBatch(ctx, ms)
}

func (t *funcTopic) Close() error {
	return nil
}

func (s *funcTopic) IsRetryable(error) bool { return false }

func (s *funcTopic) As(i interface{}) bool { return false }

func TestTopicShutdownCanBeCanceledEvenWithHangingSend(t *testing.T) {
	dt := &funcTopic{
		sendBatch: func(ctx context.Context, ms []*driver.Message) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	top := pubsub.NewTopic(dt)

	go func() {
		m := &pubsub.Message{}
		if err := top.Send(context.Background(), m); err == nil {
			t.Fatal("nil err from Send, expected context cancellation error")
		}
	}()

	// Wait long enough for top.Send() to be blocked.
	time.Sleep(10 * time.Millisecond)

	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	go func() {
		top.Shutdown(ctx)
		close(done)
	}()

	// Wait long enough for top.Shutdown() to be blocked.
	time.Sleep(10 * time.Millisecond)

	// Now cancel the context being used by top.Shutdown.
	cancel()

	// It shouldn't take too long before top.Shutdown stops.
	tooLong := 5 * time.Second
	select {
	case <-done:
	case <-time.After(tooLong):
		t.Fatalf("waited too long(%v) for Shutdown(ctx) to run", tooLong)
	}
}
