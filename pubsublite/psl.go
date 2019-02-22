package pubsublite

import (
	"gocloud.dev/pubsublite/driver"
)

type Connection struct {
	ps driver.Pubsubber
}

func (c *Connection) Publish(ctx context.Context, topic string, msg []byte) error {
	return ps.Publish(ctx, topic, msg)
}

func (c *Connection) Subscribe(ctx context.Context, callback func(msg []byte)) error {
	return ps.Subscribe(ctx, callback)
}

func (c *Connection) Close() {
	ps.Close()
}
