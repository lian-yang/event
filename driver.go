package event

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// Driver 驱动接口
type Driver interface {
	Publisher() message.Publisher
	Subscriber() message.Subscriber
	Close() error
}

// DriverOption 驱动选项函数
type DriverOption func(*driverOptions)

type driverOptions struct {
	logger        watermill.LoggerAdapter
	consumerGroup string
	consumer      string
}

// WithDriverLogger 设置日志
func WithDriverLogger(logger watermill.LoggerAdapter) DriverOption {
	return func(o *driverOptions) {
		o.logger = logger
	}
}

// WithConsumerGroup 设置消费者组
func WithConsumerGroup(group string) DriverOption {
	return func(o *driverOptions) {
		o.consumerGroup = group
	}
}

// WithConsumer 设置消费者名称
func WithConsumer(consumer string) DriverOption {
	return func(o *driverOptions) {
		o.consumer = consumer
	}
}

func applyDriverOptions(opts []DriverOption) *driverOptions {
	options := &driverOptions{
		logger:        watermill.NewStdLogger(false, false),
		consumerGroup: "events",
		consumer:      "default",
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}
