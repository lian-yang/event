package event

import (
	"context"
	"time"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr          string
	Password      string
	DB            int
	ConsumerGroup string
	Consumer      string
}

// RedisDriver Redis 驱动
type RedisDriver struct {
	client     redis.UniversalClient
	publisher  *redisstream.Publisher
	subscriber *redisstream.Subscriber
	ownClient  bool
}

// NewRedisDriver 从配置创建 Redis 驱动
func NewRedisDriver(cfg RedisConfig, opts ...DriverOption) (*RedisDriver, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	options := applyDriverOptions(opts)
	if cfg.ConsumerGroup != "" {
		options.consumerGroup = cfg.ConsumerGroup
	}
	if cfg.Consumer != "" {
		options.consumer = cfg.Consumer
	}

	driver, err := newRedisDriverFromClient(client, options)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	driver.ownClient = true
	return driver, nil
}

// NewRedisDriverFromClient 从现有客户端创建 Redis 驱动
func NewRedisDriverFromClient(client *redis.Client, opts ...DriverOption) (*RedisDriver, error) {
	options := applyDriverOptions(opts)
	return newRedisDriverFromClient(client, options)
}

// NewRedisDriverFromUniversalClient 从 UniversalClient 创建（支持集群）
func NewRedisDriverFromUniversalClient(client redis.UniversalClient, opts ...DriverOption) (*RedisDriver, error) {
	options := applyDriverOptions(opts)
	return newRedisDriverFromUniversalClient(client, options)
}

func newRedisDriverFromClient(client *redis.Client, options *driverOptions) (*RedisDriver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return createRedisDriver(client, options)
}

func newRedisDriverFromUniversalClient(client redis.UniversalClient, options *driverOptions) (*RedisDriver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return createRedisDriver(client, options)
}

func createRedisDriver(client redis.UniversalClient, options *driverOptions) (*RedisDriver, error) {
	publisher, err := redisstream.NewPublisher(
		redisstream.PublisherConfig{Client: client},
		options.logger,
	)
	if err != nil {
		return nil, err
	}

	subscriber, err := redisstream.NewSubscriber(
		redisstream.SubscriberConfig{
			Client:        client,
			ConsumerGroup: options.consumerGroup,
			Consumer:      options.consumer,
		},
		options.logger,
	)
	if err != nil {
		_ = publisher.Close()
		return nil, err
	}

	return &RedisDriver{
		client:     client,
		publisher:  publisher,
		subscriber: subscriber,
		ownClient:  false,
	}, nil
}

func (d *RedisDriver) Publisher() message.Publisher   { return d.publisher }
func (d *RedisDriver) Subscriber() message.Subscriber { return d.subscriber }

func (d *RedisDriver) Close() error {
	_ = d.publisher.Close()
	_ = d.subscriber.Close()
	if d.ownClient {
		return d.client.Close()
	}
	return nil
}
