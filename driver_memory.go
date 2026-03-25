package event

import (
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

// MemoryDriver 内存驱动
type MemoryDriver struct {
	pubSub *gochannel.GoChannel
}

// NewMemoryDriver 创建内存驱动
func NewMemoryDriver(opts ...DriverOption) *MemoryDriver {
	options := applyDriverOptions(opts)

	pubSub := gochannel.NewGoChannel(
		gochannel.Config{
			Persistent:                     true,
			BlockPublishUntilSubscriberAck: false,
		},
		options.logger,
	)

	return &MemoryDriver{pubSub: pubSub}
}

func (d *MemoryDriver) Publisher() message.Publisher   { return d.pubSub }
func (d *MemoryDriver) Subscriber() message.Subscriber { return d.pubSub }
func (d *MemoryDriver) Close() error                   { return d.pubSub.Close() }
