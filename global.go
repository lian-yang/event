package event

import (
	"context"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"go.uber.org/zap"
)

var (
	globalDispatcher *Dispatcher
	globalMu         sync.RWMutex
)

// ==================== 初始化函数 ====================

// Setup 初始化全局分发器
func Setup(driver Driver, logger watermill.LoggerAdapter) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	dispatcher, err := NewDispatcher(driver, logger)
	if err != nil {
		return err
	}

	globalDispatcher = dispatcher
	return nil
}

// SetupWithZap 使用 Zap Logger 初始化全局分发器
func SetupWithZap(driver Driver, logger *zap.Logger) error {
	return Setup(driver, NewZapLoggerAdapter(logger))
}

// SetupMemory 使用内存驱动初始化（开发环境）
func SetupMemory(logger *zap.Logger) error {
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))
	return Setup(driver, wmLogger)
}

// SetDispatcher 设置全局分发器
func SetDispatcher(dispatcher *Dispatcher) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalDispatcher = dispatcher
}

// GetDispatcher 获取全局分发器
func GetDispatcher() *Dispatcher {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalDispatcher
}

// ==================== 便捷分发函数 ====================

// Dispatch 分发事件（全局）
func Dispatch(ctx context.Context, e Event) error {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	return d.Dispatch(ctx, e)
}

// DispatchSync 同步分发事件（全局）
func DispatchSync(ctx context.Context, e Event) error {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	return d.DispatchSync(ctx, e)
}

// ==================== 便捷注册函数 ====================

// Listen 注册同步监听器（全局）
func Listen(e Event, listener Listener) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.Listen(e, listener)
}

// ListenAsync 注册异步监听器（全局）
func ListenAsync(e Event, listener Listener) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.ListenAsync(e, listener)
}

// ListenWithOptions 带选项注册监听器（全局）
func ListenWithOptions(e Event, listener Listener, opts ListenOptions) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.ListenWithOptions(e, listener, opts)
}

// ListenAsyncWithOptions 带选项注册异步监听器（全局）
func ListenAsyncWithOptions(e Event, listener Listener, opts ListenOptions) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.ListenAsyncWithOptions(e, listener, opts)
}

// ListenFunc 使用函数注册同步监听器（全局）
func ListenFunc(e Event, fn func(ctx context.Context, event Event) error) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.ListenFunc(e, fn)
}

// ListenAsyncFunc 使用函数注册异步监听器（全局）
func ListenAsyncFunc(e Event, fn func(ctx context.Context, event Event) error) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.ListenAsyncFunc(e, fn)
}

// ListenWildcard 注册通配符监听器（全局）
func ListenWildcard(pattern string, listener Listener, opts ListenOptions) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.ListenWildcard(pattern, listener, opts)
}

// ==================== 便捷配置函数 ====================

// Use 添加全局中间件
func Use(middlewares ...Middleware) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.Use(middlewares...)
}

// UseForEvent 为特定事件添加中间件
func UseForEvent(eventName string, middlewares ...Middleware) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.UseForEvent(eventName, middlewares...)
}

// SetFailedHandler 设置失败处理器
func SetFailedHandler(handler FailedJobHandler) {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	d.SetFailedHandler(handler)
}

// ==================== 运行和关闭 ====================

// Run 启动事件处理器（全局）
func Run(ctx context.Context) error {
	d := GetDispatcher()
	if d == nil {
		panic("event: dispatcher not initialized, call Setup() first")
	}
	return d.Run(ctx)
}

// Close 关闭全局分发器
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalDispatcher != nil {
		err := globalDispatcher.Close()
		globalDispatcher = nil
		return err
	}
	return nil
}

// ==================== 辅助函数 ====================

// MustDispatch 分发事件，失败时 panic
func MustDispatch(ctx context.Context, e Event) {
	if err := Dispatch(ctx, e); err != nil {
		panic(err)
	}
}

// IsInitialized 检查是否已初始化
func IsInitialized() bool {
	return GetDispatcher() != nil
}
