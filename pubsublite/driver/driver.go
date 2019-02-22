package driver

import "context"

type Pubsubber interface {
	Publish(ctx context.Context, topic string, msg []byte) error
	Subscribe(ctx context.Context, topic string, callback func(msg []byte)) error
	Close()
}
