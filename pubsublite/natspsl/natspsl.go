package natspsl

import (
	"context"

	"github.com/nats-io/go-nats"
	"gocloud.dev/pubsublite"
)

// Implements driver.Pubsubber.
type NATSConn struct {
	c *nats.Conn
}

func Connect(url string) (*pubsublite.Conn, error) {
	c, err := nats.Connect(url)
	if err != nil {
		return nil, err
	}
	nc := &NATSConn{c}
	return pubsublite.NewConn(nc), nil
}

func (c *NATSConn) Publish(ctx context.Context, topic string, msg []byte) error {
	return c.c.Publish(topic, msg)
}

func (c *NATSConn) Subscribe(ctx context.Context, topic string, callback func(msg []byte)) error {
	sub, err := c.c.Subscribe(topic, func(m *nats.Msg) {
		callback(m.Data)
	})
	if err != nil {
		return err
	}
	<-ctx.Done()
	if err := sub.Drain(); err != nil {
		return err
	}
	return ctx.Err()
}

func (c *NATSConn) Close() {
	c.c.Close()
}
