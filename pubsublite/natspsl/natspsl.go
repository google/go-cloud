package natspsl

import (
	"context"

	"github.com/nats-io/go-nats"
	"gocloud.dev/pubsublite/driver"
)

type Conn struct {
	c *nats.Conn
}

func Connect(url string) (*Conn, error) {
	c, err := nats.Connect(url)
	if err != nil {
		return nil
	}
	return &Conn{c}, nil
}

func (c *Conn) Publish(ctx context.Context, topic string, msg []byte) error {
	return c.c.Publish(topic, msg)
}

func (c *Conn) Subscribe(ctx context.Context, topic string, callback func(msg []byte)) error {
	sub, err := c.c.Subscribe(topic, func(m *nats.Message) {
		callback(m.Data)
	})
	if err != nil {
		return err
	}
	<-ctx.Done()
	if err := sub.Drain(); err != nil {
		return err
	}
	return ctx.Error()
}

func (c *Conn) Close() {
	c.c.Close()
}
