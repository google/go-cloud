package pubsub_test

import (
	"context"
	"testing"
	"time"

	"gocloud.dev/internal/pubsub"
	"gocloud.dev/internal/pubsub/driver"
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

	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	go func() {
		top.Shutdown(ctx)
		close(done)
	}()

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
