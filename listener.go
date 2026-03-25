package event

import (
	"context"
	"time"
)

// ListenerBuilder 监听器构建器
type ListenerBuilder struct {
	handler     func(ctx context.Context, event Event) error
	priority    int
	shouldQueue bool
	queue       string
	delay       time.Duration
	tries       int
	middlewares []Middleware
	stopFunc    func(ctx context.Context, event Event) bool
}

// NewListener 创建监听器构建器
func NewListener(handler func(ctx context.Context, event Event) error) *ListenerBuilder {
	return &ListenerBuilder{
		handler: handler,
		tries:   0, // 0 表示使用默认值
		queue:   DefaultQueue,
	}
}

// Priority 设置优先级
func (b *ListenerBuilder) Priority(p int) *ListenerBuilder {
	b.priority = p
	return b
}

// OnQueue 设置队列（自动启用异步）
func (b *ListenerBuilder) OnQueue(queue string) *ListenerBuilder {
	b.shouldQueue = true
	b.queue = queue
	return b
}

// Async 启用异步处理
func (b *ListenerBuilder) Async() *ListenerBuilder {
	b.shouldQueue = true
	return b
}

// Delay 设置延迟
func (b *ListenerBuilder) Delay(d time.Duration) *ListenerBuilder {
	b.delay = d
	return b
}

// Tries 设置重试次数
//
//	-1: 不重试
//	 0: 使用默认值 (DefaultTries = 3)
//	>0: 指定最大重试次数
func (b *ListenerBuilder) Tries(t int) *ListenerBuilder {
	b.tries = t
	return b
}

// NoRetry 设置不重试（等同于 Tries(-1)）
func (b *ListenerBuilder) NoRetry() *ListenerBuilder {
	return b.Tries(NoRetry)
}

// Middleware 添加中间件
func (b *ListenerBuilder) Middleware(m ...Middleware) *ListenerBuilder {
	b.middlewares = append(b.middlewares, m...)
	return b
}

// StopWhen 设置停止条件
func (b *ListenerBuilder) StopWhen(fn func(ctx context.Context, event Event) bool) *ListenerBuilder {
	b.stopFunc = fn
	return b
}

// Build 构建监听器
func (b *ListenerBuilder) Build() *BuiltListener {
	return &BuiltListener{
		handler:     b.handler,
		priority:    b.priority,
		shouldQueue: b.shouldQueue,
		queue:       b.queue,
		delay:       b.delay,
		tries:       b.tries,
		middlewares: b.middlewares,
		stopFunc:    b.stopFunc,
	}
}

// BuiltListener 构建完成的监听器
type BuiltListener struct {
	handler     func(ctx context.Context, event Event) error
	priority    int
	shouldQueue bool
	queue       string
	delay       time.Duration
	tries       int
	middlewares []Middleware
	stopFunc    func(ctx context.Context, event Event) bool
}

func (l *BuiltListener) Handle(ctx context.Context, event Event) error {
	return l.handler(ctx, event)
}

func (l *BuiltListener) Priority() int {
	return l.priority
}

func (l *BuiltListener) ShouldQueue() bool {
	return l.shouldQueue
}

func (l *BuiltListener) Queue() string {
	return l.queue
}

func (l *BuiltListener) Delay() time.Duration {
	return l.delay
}

func (l *BuiltListener) Tries() int {
	return l.tries
}

func (l *BuiltListener) Middleware() []Middleware {
	return l.middlewares
}

func (l *BuiltListener) ShouldStop(ctx context.Context, event Event) bool {
	if l.stopFunc != nil {
		return l.stopFunc(ctx, event)
	}
	return false
}
