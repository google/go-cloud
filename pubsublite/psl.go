package pubsublite

import (
	"context"

	"gocloud.dev/pubsublite/driver"
)

type Conn struct {
	ps driver.Pubsubber
}

func NewConn(ps driver.Pubsubber) *Conn {
	return &Conn{ps}
}

func (c *Conn) Publish(ctx context.Context, topic string, msg []byte) error {
	return c.ps.Publish(ctx, topic, msg)
}

func (c *Conn) Subscribe(ctx context.Context, topic string, callback func(msg []byte)) error {
	return c.ps.Subscribe(ctx, topic, callback)
}

func (c *Conn) Close() {
	c.ps.Close()
}
