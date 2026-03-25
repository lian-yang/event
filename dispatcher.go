package event

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"go.uber.org/zap"
)

// ==================== 监听器条目 ====================

type listenerEntry struct {
	name        string
	listener    Listener
	priority    int
	async       bool
	queue       string
	middlewares []Middleware
}

type queueListenerEntry struct {
	name        string
	listener    Listener
	queue       string
	delay       time.Duration
	tries       int
	middlewares []Middleware
}

type wildcardEntry struct {
	pattern  string
	listener *listenerEntry
}

// ==================== 分发器 ====================

// Dispatcher 事件分发器
type Dispatcher struct {
	driver            Driver
	router            *message.Router
	wmLogger          watermill.LoggerAdapter
	zapLogger         *zap.Logger
	listeners         map[string][]*listenerEntry
	wildcardListeners []*wildcardEntry
	queueListeners    map[string]*queueListenerEntry
	eventTypes        map[string]reflect.Type
	globalMiddlewares []Middleware
	eventMiddlewares  map[string][]Middleware
	failedHandler     FailedJobHandler
	mu                sync.RWMutex
	running           bool
}

// NewDispatcher 创建分发器
func NewDispatcher(driver Driver, wmLogger watermill.LoggerAdapter) (*Dispatcher, error) {
	router, err := message.NewRouter(message.RouterConfig{}, wmLogger)
	if err != nil {
		return nil, err
	}

	var zapLogger *zap.Logger
	if adapter, ok := wmLogger.(*ZapLoggerAdapter); ok {
		zapLogger = adapter.GetZapLogger()
	}

	return &Dispatcher{
		driver:            driver,
		router:            router,
		wmLogger:          wmLogger,
		zapLogger:         zapLogger,
		listeners:         make(map[string][]*listenerEntry),
		wildcardListeners: make([]*wildcardEntry, 0),
		queueListeners:    make(map[string]*queueListenerEntry),
		eventTypes:        make(map[string]reflect.Type),
		eventMiddlewares:  make(map[string][]Middleware),
	}, nil
}

// ==================== 日志辅助 ====================

func (d *Dispatcher) logDebug(msg string, fields ...zap.Field) {
	if d.zapLogger != nil {
		d.zapLogger.Debug(msg, fields...)
	}
}

func (d *Dispatcher) logInfo(msg string, fields ...zap.Field) {
	if d.zapLogger != nil {
		d.zapLogger.Info(msg, fields...)
	}
}

func (d *Dispatcher) logError(msg string, err error, fields ...zap.Field) {
	if d.zapLogger != nil {
		fields = append(fields, zap.Error(err))
		d.zapLogger.Error(msg, fields...)
	}
}

// ==================== 配置方法 ====================

// Use 添加全局中间件
func (d *Dispatcher) Use(middlewares ...Middleware) *Dispatcher {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.globalMiddlewares = append(d.globalMiddlewares, middlewares...)
	return d
}

// UseForEvent 为特定事件添加中间件
func (d *Dispatcher) UseForEvent(eventName string, middlewares ...Middleware) *Dispatcher {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.eventMiddlewares[eventName] = append(d.eventMiddlewares[eventName], middlewares...)
	return d
}

// SetFailedHandler 设置失败处理器
func (d *Dispatcher) SetFailedHandler(handler FailedJobHandler) *Dispatcher {
	d.failedHandler = handler
	return d
}

// ==================== 注册监听器 ====================

// ListenOptions 监听选项
type ListenOptions struct {
	Name     string
	Priority int
	Async    bool
	Queue    string
	// Delay 延迟时间
	// 使用 time.Duration 类型，例如: time.Second * 10
	Delay time.Duration
	// Tries 重试次数
	//   -1: 不重试
	//    0: 使用默认值 (DefaultTries = 3)
	//   >0: 指定最大重试次数
	Tries       int
	Middlewares []Middleware
}

// Listen 注册同步监听器
func (d *Dispatcher) Listen(event Event, listener Listener) *Dispatcher {
	return d.ListenWithOptions(event, listener, ListenOptions{})
}

// ListenWithOptions 带选项注册监听器
func (d *Dispatcher) ListenWithOptions(event Event, listener Listener, opts ListenOptions) *Dispatcher {
	d.mu.Lock()
	defer d.mu.Unlock()

	eventName := event.EventName()

	// 生成监听器名称
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("%s_%s_%d", eventName, reflect.TypeOf(listener).String(), len(d.listeners[eventName]))
	}

	// 获取优先级
	priority := opts.Priority
	if pl, ok := listener.(PriorityListener); ok {
		priority = pl.Priority()
	}

	// 收集中间件
	middlewares := opts.Middlewares
	if ml, ok := listener.(MiddlewareAwareListener); ok {
		middlewares = append(middlewares, ml.Middleware()...)
	}

	async := opts.Async
	queue := opts.Queue
	tries := opts.Tries
	delay := opts.Delay

	// 从监听器接口获取配置
	if ql, ok := listener.(QueueableListener); ok {
		if ql.ShouldQueue() {
			async = true
		}

		// Queue：opts 优先，其次监听器配置
		if queue == "" {
			queue = ql.Queue()
		}

		// Delay: opts 优先（opts.Delay > 0 表示已设置）
		if delay == 0 {
			delay = ql.Delay()
		}

		// Tries：opts.Tries != 0 时使用 opts，否则使用监听器配置
		// 注意：opts.Tries == 0 表示未设置，opts.Tries == -1 表示不重试
		if tries == 0 {
			tries = ql.Tries()
		}
	}

	if queue == "" {
		queue = DefaultQueue
	}

	// 解析最终的 Tries 值
	// -1 -> -1 (不重试)
	//  0 -> DefaultTries (使用默认值)
	// >0 -> tries (使用指定值)
	resolvedTries := ResolveTries(tries)

	entry := &listenerEntry{
		name:        opts.Name,
		listener:    listener,
		priority:    priority,
		async:       async,
		queue:       queue,
		middlewares: middlewares,
	}

	// 检查是否是通配符模式
	if strings.Contains(eventName, "*") {
		d.wildcardListeners = append(d.wildcardListeners, &wildcardEntry{
			pattern:  eventName,
			listener: entry,
		})
	} else {
		d.listeners[eventName] = append(d.listeners[eventName], entry)
		d.sortListeners(eventName)
	}

	// 注册事件类型
	d.eventTypes[eventName] = reflect.TypeOf(event).Elem()

	// 如果是异步监听器，注册到队列监听器
	if async {
		d.queueListeners[opts.Name] = &queueListenerEntry{
			name:        opts.Name,
			listener:    listener,
			queue:       queue,
			delay:       opts.Delay,
			tries:       resolvedTries,
			middlewares: middlewares,
		}

		// 调试日志
		var triesInfo string
		if resolvedTries == NoRetry {
			triesInfo = "no-retry"
		} else {
			triesInfo = fmt.Sprintf("max %d", resolvedTries)
		}
		d.logDebug("registered async listener",
			zap.String("name", opts.Name),
			zap.String("event", eventName),
			zap.String("queue", queue),
			zap.Duration("delay", delay),
			zap.String("tries", triesInfo),
		)
	}

	return d
}

// ListenAsync 注册异步监听器
func (d *Dispatcher) ListenAsync(event Event, listener Listener) *Dispatcher {
	return d.ListenWithOptions(event, listener, ListenOptions{Async: true})
}

// ListenAsyncWithOptions 注册异步监听器（带选项）
func (d *Dispatcher) ListenAsyncWithOptions(event Event, listener Listener, opts ListenOptions) *Dispatcher {
	opts.Async = true
	return d.ListenWithOptions(event, listener, opts)
}

// ListenFunc 使用函数注册监听器
func (d *Dispatcher) ListenFunc(event Event, fn func(ctx context.Context, event Event) error) *Dispatcher {
	return d.Listen(event, ListenerFunc(fn))
}

// ListenAsyncFunc 使用函数注册异步监听器
func (d *Dispatcher) ListenAsyncFunc(event Event, fn func(ctx context.Context, event Event) error) *Dispatcher {
	return d.ListenAsync(event, ListenerFunc(fn))
}

// ==================== 通配符监听 ====================

// ListenWildcard 注册通配符监听器
func (d *Dispatcher) ListenWildcard(pattern string, listener Listener, opts ListenOptions) *Dispatcher {
	d.mu.Lock()
	defer d.mu.Unlock()

	if opts.Name == "" {
		opts.Name = fmt.Sprintf("wildcard_%s_%d", pattern, len(d.wildcardListeners))
	}

	priority := opts.Priority
	if pl, ok := listener.(PriorityListener); ok {
		priority = pl.Priority()
	}

	middlewares := opts.Middlewares
	if ml, ok := listener.(MiddlewareAwareListener); ok {
		middlewares = append(middlewares, ml.Middleware()...)
	}

	queue := opts.Queue
	if queue == "" {
		queue = DefaultQueue
	}

	entry := &listenerEntry{
		name:        opts.Name,
		listener:    listener,
		priority:    priority,
		async:       opts.Async,
		queue:       queue,
		middlewares: middlewares,
	}

	d.wildcardListeners = append(d.wildcardListeners, &wildcardEntry{
		pattern:  pattern,
		listener: entry,
	})

	if opts.Async {
		d.queueListeners[opts.Name] = &queueListenerEntry{
			name:        opts.Name,
			listener:    listener,
			queue:       queue,
			delay:       opts.Delay,
			tries:       opts.Tries,
			middlewares: middlewares,
		}
	}

	return d
}

// matchWildcard 匹配通配符
func matchWildcard(pattern, eventName string) bool {
	if pattern == "*" {
		return true
	}

	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		prefix := parts[0]
		suffix := parts[1]

		if prefix != "" && suffix != "" {
			return strings.HasPrefix(eventName, prefix) && strings.HasSuffix(eventName, suffix)
		}
		if prefix != "" {
			return strings.HasPrefix(eventName, prefix)
		}
		if suffix != "" {
			return strings.HasSuffix(eventName, suffix)
		}
	}

	return pattern == eventName
}

// getWildcardListeners 获取匹配的通配符监听器
func (d *Dispatcher) getWildcardListeners(eventName string) []*listenerEntry {
	var matched []*listenerEntry
	for _, wl := range d.wildcardListeners {
		if matchWildcard(wl.pattern, eventName) {
			matched = append(matched, wl.listener)
		}
	}
	return matched
}

// sortListeners 按优先级排序监听器
func (d *Dispatcher) sortListeners(eventName string) {
	entries := d.listeners[eventName]
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})
}

// ==================== 分发事件 ====================

// Dispatch 分发事件
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) error {
	return d.dispatch(ctx, event, true)
}

// DispatchSync 同步分发事件（忽略队列设置）
func (d *Dispatcher) DispatchSync(ctx context.Context, event Event) error {
	return d.dispatch(ctx, event, false)
}

func (d *Dispatcher) dispatch(ctx context.Context, event Event, allowAsync bool) error {
	eventName := event.EventName()

	d.mu.RLock()
	entries := make([]*listenerEntry, len(d.listeners[eventName]))
	copy(entries, d.listeners[eventName])

	// 添加匹配的通配符监听器
	wildcardEntries := d.getWildcardListeners(eventName)
	entries = append(entries, wildcardEntries...)

	// 注册事件类型
	if _, ok := d.eventTypes[eventName]; !ok {
		d.eventTypes[eventName] = reflect.TypeOf(event).Elem()
	}

	globalMiddlewares := d.globalMiddlewares
	eventMiddlewares := d.eventMiddlewares[eventName]
	d.mu.RUnlock()

	if len(entries) == 0 {
		return nil
	}

	// 按优先级排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	// 处理监听器
	for _, entry := range entries {
		if allowAsync && entry.async {
			if err := d.dispatchToQueue(ctx, event, entry); err != nil {
				d.logError("dispatch to queue failed", err,
					zap.String("event", eventName),
					zap.String("listener", entry.name),
				)
			}
			continue
		}

		// 构建中间件链
		chain := NewMiddlewareChain(globalMiddlewares...)
		chain.Append(eventMiddlewares...)
		chain.Append(entry.middlewares...)

		handler := chain.Then(func(ctx context.Context, e Event) error {
			return entry.listener.Handle(ctx, e)
		})

		if err := handler(ctx, event); err != nil {
			d.logError("listener error", err,
				zap.String("event", eventName),
				zap.String("listener", entry.name),
			)
		}

		// 检查是否停止传播
		if sl, ok := entry.listener.(StoppableListener); ok {
			if sl.ShouldStop(ctx, event) {
				d.logDebug("event propagation stopped",
					zap.String("event", eventName),
					zap.String("listener", entry.name),
				)
				break
			}
		}
	}

	return nil
}

// dispatchToQueue 发布到队列
func (d *Dispatcher) dispatchToQueue(ctx context.Context, event Event, entry *listenerEntry) error {
	// 获取该监听器的配置
	delay, tries := d.getListenerConfig(entry)

	msg, err := d.createJobMessage(event, entry.name, delay, tries)
	if err != nil {
		return err
	}

	queue := entry.queue
	if queue == "" {
		queue = d.getListenerQueue(entry.name, event)
	}

	if err := d.driver.Publisher().Publish(queue, msg); err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	// 日志
	var triesInfo string
	if tries == NoRetry {
		triesInfo = "no-retry"
	} else {
		triesInfo = fmt.Sprintf("max %d", tries)
	}

	d.logDebug("event queued",
		zap.String("event", event.EventName()),
		zap.String("listener", entry.name),
		zap.String("queue", queue),
		zap.Duration("delay", delay),
		zap.String("tries", triesInfo),
	)

	return nil
}

// getListenerConfig 获取监听器的 delay 和 tries 配置
func (d *Dispatcher) getListenerConfig(entry *listenerEntry) (delay time.Duration, tries int) {
	// 从已注册的队列监听器获取（已解析过的值）
	if qe, ok := d.queueListeners[entry.name]; ok {
		return qe.delay, qe.tries
	}

	// 从监听器接口获取
	if ql, ok := entry.listener.(QueueableListener); ok {
		delay = ql.Delay()
		tries = ResolveTries(ql.Tries())
		return
	}

	// 默认值
	return 0, DefaultTries
}

// ==================== 启动和关闭 ====================

// Run 启动事件处理器
func (d *Dispatcher) Run(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = true

	// 收集所有队列
	queues := make(map[string]bool)
	for _, entry := range d.queueListeners {
		queue := entry.queue
		if queue == "" {
			queue = DefaultQueue
		}
		queues[queue] = true
	}

	// 为每个队列注册消费者
	for queue := range queues {
		queueName := queue
		d.router.AddNoPublisherHandler(
			queueName+"_consumer",
			queueName,
			d.driver.Subscriber(),
			func(msg *message.Message) error {
				return d.processJob(msg)
			},
		)
		d.logInfo("queue consumer registered", zap.String("queue", queueName))
	}

	d.mu.Unlock()

	d.logInfo("event dispatcher started")
	return d.router.Run(ctx)
}

// Close 关闭分发器
func (d *Dispatcher) Close() error {
	d.logInfo("closing event dispatcher")

	var errs []error
	if d.router != nil {
		if err := d.router.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if d.driver != nil {
		if err := d.driver.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
