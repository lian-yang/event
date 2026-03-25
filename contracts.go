package event

import (
	"context"
	"time"
)

// ==================== 事件接口 ====================

// Event 基础事件接口
type Event interface {
	EventName() string
}

// ShouldQueue 标记事件需要队列处理
type ShouldQueue interface {
	Event
	Queue() string
}

// Delayable 可延迟的事件
type Delayable interface {
	Event
	Delay() time.Duration
}

// Retryable 可重试的事件
type Retryable interface {
	Event
	// Tries 返回重试配置
	//   -1: 不重试，失败直接标记失败
	//    0: 使用默认重试次数 (DefaultTries = 3)
	//   >0: 指定最大重试次数
	Tries() int
	// RetryDelay 重试间隔
	RetryDelay() time.Duration
}

// ==================== 监听器接口 ====================

// Listener 基础监听器接口
type Listener interface {
	Handle(ctx context.Context, event Event) error
}

// ListenerFunc 函数类型监听器
type ListenerFunc func(ctx context.Context, event Event) error

func (f ListenerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// PriorityListener 带优先级的监听器
type PriorityListener interface {
	Listener
	Priority() int
}

// StoppableListener 可停止传播的监听器
type StoppableListener interface {
	Listener
	ShouldStop(ctx context.Context, event Event) bool
}

// QueueableListener 可排队的监听器
type QueueableListener interface {
	Listener
	ShouldQueue() bool
	Queue() string
	// Delay 返回延迟时间
	//   0: 无延迟，立即执行
	//   >0: 延迟指定时间后执行
	Delay() time.Duration
	// Tries 返回重试配置
	//   -1: 不重试
	//    0: 使用默认重试次数
	//   >0: 指定最大重试次数
	Tries() int
}

// MiddlewareAwareListener 带中间件的监听器
type MiddlewareAwareListener interface {
	Listener
	Middleware() []Middleware
}

// ==================== 中间件接口 ====================

// Handler 处理器函数类型
type Handler func(ctx context.Context, event Event) error

// Middleware 中间件接口
type Middleware interface {
	Handle(ctx context.Context, event Event, next Handler) error
}

// MiddlewareFunc 函数类型中间件
type MiddlewareFunc func(ctx context.Context, event Event, next Handler) error

func (f MiddlewareFunc) Handle(ctx context.Context, event Event, next Handler) error {
	return f(ctx, event, next)
}

// ==================== 队列任务 ====================

// Job 队列任务
type Job struct {
	ID           string        `json:"id"`
	EventName    string        `json:"event_name"`
	EventType    string        `json:"event_type"`
	ListenerName string        `json:"listener_name"`
	Payload      []byte        `json:"payload"`
	Queue        string        `json:"queue"`
	Delay        time.Duration `json:"delay"`
	Tries        int           `json:"tries"`
	Attempts     int           `json:"attempts"`
	LastError    string        `json:"last_error,omitempty"`
	AvailableAt  time.Time     `json:"available_at"`
	CreatedAt    time.Time     `json:"created_at"`
}

// ==================== 失败处理 ====================

// FailedJobHandler 失败任务处理器
type FailedJobHandler interface {
	Handle(ctx context.Context, job *Job, err error) error
}

// FailedJobHandlerFunc 函数类型失败处理器
type FailedJobHandlerFunc func(ctx context.Context, job *Job, err error) error

func (f FailedJobHandlerFunc) Handle(ctx context.Context, job *Job, err error) error {
	return f(ctx, job, err)
}

// ==================== 订阅者接口 ====================

// Subscriber 事件订阅者
type Subscriber interface {
	Subscribe() map[string][]Listener
}
