package event

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"go.uber.org/zap/zaptest"
)

// TestNewDispatcher 测试创建分发器
func TestNewDispatcher(t *testing.T) {
	t.Run("creates dispatcher with memory driver", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)

		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, err := NewDispatcher(driver, wmLogger)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if dispatcher == nil {
			t.Fatal("expected dispatcher to be created")
		}
		if dispatcher.driver == nil {
			t.Error("expected driver to be set")
		}
		if dispatcher.router == nil {
			t.Error("expected router to be set")
		}
		if dispatcher.listeners == nil {
			t.Error("expected listeners map to be initialized")
		}
		if dispatcher.eventTypes == nil {
			t.Error("expected eventTypes map to be initialized")
		}

		// 清理
		_ = dispatcher.Close()
	})
}

// TestDispatcher_Listen 测试监听器注册
func TestDispatcher_Listen(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("registers synchronous listener", func(t *testing.T) {
		event := &testUserRegistered{UserID: 1}
		listener := &testSendWelcomeEmail{}

		dispatcher.Listen(event, listener)

		if len(dispatcher.listeners["user.registered"]) != 1 {
			t.Errorf("expected 1 listener, got %d", len(dispatcher.listeners["user.registered"]))
		}
	})

	t.Run("registers multiple listeners for same event", func(t *testing.T) {
		event := &testOrderPaid{OrderID: 123}
		listener1 := &testLogOrder{}
		listener2 := &testUpdateInventory{}

		dispatcher.Listen(event, listener1)
		dispatcher.Listen(event, listener2)

		if len(dispatcher.listeners["order.paid"]) != 2 {
			t.Errorf("expected 2 listeners, got %d", len(dispatcher.listeners["order.paid"]))
		}
	})

	t.Run("sorts listeners by priority", func(t *testing.T) {
		event := &testPriorityEvent{}

		dispatcher.ListenWithOptions(event, &testLowPriorityListener{}, ListenOptions{
			Name:     "low",
			Priority: 10,
		})
		dispatcher.ListenWithOptions(event, &testHighPriorityListener{}, ListenOptions{
			Name:     "high",
			Priority: 100,
		})
		dispatcher.ListenWithOptions(event, &testMediumPriorityListener{}, ListenOptions{
			Name:     "medium",
			Priority: 50,
		})

		entries := dispatcher.listeners["priority.test"]
		if len(entries) != 3 {
			t.Fatalf("expected 3 listeners, got %d", len(entries))
		}

		// 验证按优先级降序排列 (high -> medium -> low)
		if entries[0].priority != 100 {
			t.Errorf("expected first priority 100, got %d", entries[0].priority)
		}
		if entries[1].priority != 50 {
			t.Errorf("expected second priority 50, got %d", entries[1].priority)
		}
		if entries[2].priority != 10 {
			t.Errorf("expected third priority 10, got %d", entries[2].priority)
		}
	})
}

// TestDispatcher_ListenAsync 测试异步监听器注册
func TestDispatcher_ListenAsync(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("registers async listener", func(t *testing.T) {
		event := &testUserRegistered{UserID: 1}
		listener := &testQueueableListener{}

		dispatcher.ListenAsync(event, listener)

		entries := dispatcher.listeners["user.registered"]
		if len(entries) != 1 {
			t.Fatalf("expected 1 listener, got %d", len(entries))
		}

		if !entries[0].async {
			t.Error("expected listener to be async")
		}
	})

	t.Run("registers to queueListeners map", func(t *testing.T) {
		event := &testUserRegistered{UserID: 2}
		listener := &testQueueableListener{}

		dispatcher.ListenAsync(event, listener)

		// 验证queueListeners中有记录
		if len(dispatcher.queueListeners) == 0 {
			t.Error("expected queueListeners to have entries")
		}
	})
}

// TestDispatcher_Dispatch 测试事件分发
func TestDispatcher_Dispatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("dispatches event to single listener", func(t *testing.T) {
		var mu sync.Mutex
		called := false

		event := &testUserRegistered{UserID: 1}
		listener := ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		})

		dispatcher.Listen(event, listener)

		err := dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		wasCalled := called
		mu.Unlock()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !wasCalled {
			t.Error("expected listener to be called")
		}
	})

	t.Run("dispatches event to multiple listeners in priority order", func(t *testing.T) {
		var mu sync.Mutex
		var order []string

		event := &testPriorityEvent{}

		dispatcher.ListenWithOptions(event, ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			order = append(order, "low")
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "low", Priority: 10})

		dispatcher.ListenWithOptions(event, ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			order = append(order, "high")
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "high", Priority: 100})

		dispatcher.ListenWithOptions(event, ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			order = append(order, "medium")
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "medium", Priority: 50})

		err := dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		executionOrder := order
		mu.Unlock()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := []string{"high", "medium", "low"}
		if len(executionOrder) != len(expected) {
			t.Errorf("expected %d executions, got %d", len(expected), len(executionOrder))
		}

		for i, v := range expected {
			if i >= len(executionOrder) || executionOrder[i] != v {
				t.Errorf("expected order[%d] = %s, got %v", i, v, executionOrder)
			}
		}
	})

	t.Run("handles listener error gracefully", func(t *testing.T) {
		event := &testUserRegistered{UserID: 1}

		listener := ListenerFunc(func(ctx context.Context, e Event) error {
			return errors.New("listener error")
		})

		dispatcher.Listen(event, listener)

		// Dispatch should not return error even if listener fails
		err := dispatcher.Dispatch(context.Background(), event)

		if err != nil {
			t.Errorf("expected no error from Dispatch (listener errors are logged), got %v", err)
		}
	})

	t.Run("stops propagation when ShouldStop returns true", func(t *testing.T) {
		var mu sync.Mutex
		var order []string

		event := &testUserRegistered{UserID: 1}

		// 第一个监听器返回ShouldStop=true
		dispatcher.ListenWithOptions(event, &testStoppableListener{
			shouldStop: true,
			handleFunc: func(ctx context.Context, e Event) error {
				mu.Lock()
				order = append(order, "stopper")
				mu.Unlock()
				return nil
			},
		}, ListenOptions{Name: "stopper", Priority: 100})

		// 第二个监听器不应该被调用
		dispatcher.ListenWithOptions(event, ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			order = append(order, "after")
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "after", Priority: 50})

		_ = dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		executionOrder := order
		mu.Unlock()

		if len(executionOrder) != 1 {
			t.Errorf("expected 1 execution (propagation stopped), got %d", len(executionOrder))
		}
		if len(executionOrder) > 0 && executionOrder[0] != "stopper" {
			t.Errorf("expected stopper to be called, got %s", executionOrder[0])
		}
	})
}

// TestDispatcher_Wildcard 测试通配符监听器
func TestDispatcher_Wildcard(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("matches single asterisk pattern", func(t *testing.T) {
		var mu sync.Mutex
		called := false

		dispatcher.ListenWildcard("*", ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "catch-all"})

		event := &testUserRegistered{UserID: 1}
		_ = dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		wasCalled := called
		mu.Unlock()

		if !wasCalled {
			t.Error("expected wildcard listener to be called")
		}
	})

	t.Run("matches prefix pattern", func(t *testing.T) {
		var mu sync.Mutex
		var matchedEvents []string

		dispatcher.ListenWildcard("user.*", ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			matchedEvents = append(matchedEvents, e.EventName())
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "user-wildcard"})

		_ = dispatcher.Dispatch(context.Background(), &testUserRegistered{UserID: 1})
		_ = dispatcher.Dispatch(context.Background(), &testUserUpdated{UserID: 2})
		_ = dispatcher.Dispatch(context.Background(), &testOrderPaid{OrderID: 123}) // 不应该匹配

		mu.Lock()
		events := matchedEvents
		mu.Unlock()

		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("matches suffix pattern", func(t *testing.T) {
		var mu sync.Mutex
		var matchedEvents []string

		dispatcher.ListenWildcard("*.created", ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			matchedEvents = append(matchedEvents, e.EventName())
			mu.Unlock()
			return nil
		}), ListenOptions{Name: "created-wildcard"})

		_ = dispatcher.Dispatch(context.Background(), &testUserCreated{ID: 1})
		_ = dispatcher.Dispatch(context.Background(), &testOrderCreated{ID: 2})
		_ = dispatcher.Dispatch(context.Background(), &testUserUpdated{UserID: 3}) // 不应该匹配

		mu.Lock()
		events := matchedEvents
		mu.Unlock()

		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})
}

// TestDispatcher_Middleware 测试中间件
func TestDispatcher_Middleware(t *testing.T) {
	t.Run("applies global middleware", func(t *testing.T) {
		// 创建新的dispatcher
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, err := NewDispatcher(driver, wmLogger)
		if err != nil {
			t.Fatalf("failed to create dispatcher: %v", err)
		}
		defer dispatcher.Close()

		var mu sync.Mutex
		var order []string

		globalMW := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			mu.Lock()
			order = append(order, "global-before")
			mu.Unlock()
			err := next(ctx, event)
			mu.Lock()
			order = append(order, "global-after")
			mu.Unlock()
			return err
		})

		dispatcher.Use(globalMW)

		listener := ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			order = append(order, "listener")
			mu.Unlock()
			return nil
		})

		event := &testUserRegistered{UserID: 1}
		dispatcher.Listen(event, listener)

		_ = dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		executionOrder := order
		mu.Unlock()

		expected := []string{"global-before", "listener", "global-after"}
		if len(executionOrder) != len(expected) {
			t.Errorf("expected %d entries, got %d", len(expected), len(executionOrder))
		}

		for i, v := range expected {
			if i >= len(executionOrder) || executionOrder[i] != v {
				t.Errorf("expected order[%d] = %s, got %v", i, v, executionOrder)
			}
		}
	})

	t.Run("applies event-level middleware", func(t *testing.T) {
		// 创建新的dispatcher以避免全局中间件影响
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, err := NewDispatcher(driver, wmLogger)
		if err != nil {
			t.Fatalf("failed to create dispatcher: %v", err)
		}
		defer dispatcher.Close()

		var mu sync.Mutex
		var order []string

		eventMW := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			mu.Lock()
			order = append(order, "event-mw")
			mu.Unlock()
			return next(ctx, event)
		})

		dispatcher.UseForEvent("user.registered", eventMW)

		listener := ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			order = append(order, "listener")
			mu.Unlock()
			return nil
		})

		event := &testUserRegistered{UserID: 1}
		dispatcher.Listen(event, listener)

		_ = dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		executionOrder := order
		mu.Unlock()

		if len(executionOrder) != 2 {
			t.Errorf("expected 2 entries, got %d: %v", len(executionOrder), executionOrder)
		}
	})
}

// TestDispatcher_DispatchSync 测试同步分发
func TestDispatcher_DispatchSync(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("dispatches synchronously ignoring async flag", func(t *testing.T) {
		var mu sync.Mutex
		called := false

		listener := ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		})

		event := &testUserRegistered{UserID: 1}

		// Register with async flag
		dispatcher.ListenAsync(event, listener)

		// DispatchSync should still execute synchronously
		err := dispatcher.DispatchSync(context.Background(), event)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		mu.Lock()
		wasCalled := called
		mu.Unlock()

		if !wasCalled {
			t.Error("expected listener to be called synchronously")
		}

		_ = wasCalled // 使用变量避免编译器警告
	})
}

// TestDispatcher_ListenFunc 测试函数监听器
func TestDispatcher_ListenFunc(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("registers function listener", func(t *testing.T) {
		var mu sync.Mutex
		called := false

		event := &testUserRegistered{UserID: 1}
		dispatcher.ListenFunc(event, func(ctx context.Context, e Event) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		})

		_ = dispatcher.Dispatch(context.Background(), event)

		mu.Lock()
		wasCalled := called
		mu.Unlock()

		if !wasCalled {
			t.Error("expected listener to be called")
		}
	})
}

// TestDispatcher_Run 测试运行队列消费者
func TestDispatcher_Run(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("starts queue consumers", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Register an async listener
		event := &testUserRegistered{UserID: 1}
		listener := &testQueueableListener{}
		dispatcher.ListenAsync(event, listener)

		// Run should start without error
		errCh := make(chan error, 1)
		go func() {
			errCh <- dispatcher.Run(ctx)
		}()

		// Give it time to start
		time.Sleep(time.Millisecond * 100)

		// Cancel to stop
		cancel()

		select {
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run did not stop after context cancellation")
		}
	})
}

// TestDispatcher_Close 测试关闭分发器
func TestDispatcher_Close(t *testing.T) {
	t.Run("closes without error", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, err := NewDispatcher(driver, wmLogger)
		if err != nil {
			t.Fatalf("failed to create dispatcher: %v", err)
		}

		err = dispatcher.Close()
		if err != nil {
			t.Errorf("expected no error on close, got %v", err)
		}
	})

	t.Run("can be closed multiple times", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, _ := NewDispatcher(driver, wmLogger)

		_ = dispatcher.Close()
		_ = dispatcher.Close() // Should not panic or error
	})
}

// TestDispatcher_FailedHandler 测试失败处理器
func TestDispatcher_FailedHandler(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("sets custom failed handler", func(t *testing.T) {
		_ = false // was called flag
		handler := FailedJobHandlerFunc(func(ctx context.Context, job *Job, err error) error {
			return nil
		})

		result := dispatcher.SetFailedHandler(handler)

		if result != dispatcher {
			t.Error("expected SetFailedHandler to return dispatcher for chaining")
		}

		// Verify it was set
		if dispatcher.failedHandler == nil {
			t.Error("expected failedHandler to be set")
		}
	})
}

// TestDispatcher_GetListenerConfig 测试获取监听器配置
func TestDispatcher_GetListenerConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	wmLogger := NewZapLoggerAdapter(logger)
	driver := NewMemoryDriver(WithDriverLogger(wmLogger))

	dispatcher, err := NewDispatcher(driver, wmLogger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}
	defer dispatcher.Close()

	t.Run("gets config from queue listener entry", func(t *testing.T) {
		delay := time.Second * 5
		tries := 5

		event := &testUserRegistered{UserID: 1}
		listener := &testQueueableListener{}

		dispatcher.ListenAsync(event, listener)

		// Get the entry name
		entries := dispatcher.listeners["user.registered"]
		if len(entries) == 0 {
			t.Fatal("expected listener to be registered")
		}

		entryName := entries[0].name

		// Register in queueListeners with custom config
		dispatcher.queueListeners[entryName] = &queueListenerEntry{
			name:     entryName,
			listener: listener,
			delay:    delay,
			tries:    tries,
		}

		gotDelay, gotTries := dispatcher.getListenerConfig(entries[0])

		if gotDelay != delay {
			t.Errorf("expected delay %v, got %v", delay, gotDelay)
		}
		if gotTries != tries {
			t.Errorf("expected tries %d, got %d", tries, gotTries)
		}
	})

	t.Run("returns defaults when not found", func(t *testing.T) {
		entry := &listenerEntry{
			name:     "test",
			listener: &testSendWelcomeEmail{},
		}

		gotDelay, gotTries := dispatcher.getListenerConfig(entry)

		if gotDelay != 0 {
			t.Errorf("expected delay 0, got %v", gotDelay)
		}
		if gotTries != DefaultTries {
			t.Errorf("expected tries %d, got %d", DefaultTries, gotTries)
		}
	})
}

// Test_matchWildcard 测试通配符匹配函数
func Test_matchWildcard(t *testing.T) {
	tests := []struct {
		pattern   string
		eventName string
		expected  bool
	}{
		{"*", "any.event", true},
		{"user.*", "user.created", true},
		{"user.*", "user.updated", true},
		{"user.*", "order.created", false},
		{"*.created", "user.created", true},
		{"*.created", "order.created", true},
		{"*.created", "user.updated", false},
		{"user.*.created", "user.profile.created", true},
		{"exact.match", "exact.match", true},
		{"exact.match", "exact.noMatch", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.eventName, func(t *testing.T) {
			result := matchWildcard(tt.pattern, tt.eventName)
			if result != tt.expected {
				t.Errorf("matchWildcard(%s, %s) = %v, expected %v",
					tt.pattern, tt.eventName, result, tt.expected)
			}
		})
	}
}

// ==================== 测试辅助类型 ====================

type testUserRegistered struct {
	UserID int64
}

func (e *testUserRegistered) EventName() string { return "user.registered" }

type testUserUpdated struct {
	UserID int64
}

func (e *testUserUpdated) EventName() string { return "user.updated" }

type testUserCreated struct {
	ID int64
}

func (e *testUserCreated) EventName() string { return "user.created" }

type testOrderCreated struct {
	ID int64
}

func (e *testOrderCreated) EventName() string { return "order.created" }

type testOrderPaid struct {
	OrderID int64
}

func (e *testOrderPaid) EventName() string { return "order.paid" }

type testPriorityEvent struct{}

func (e *testPriorityEvent) EventName() string { return "priority.test" }

type testSendWelcomeEmail struct{}

func (l *testSendWelcomeEmail) Handle(ctx context.Context, event Event) error {
	return nil
}

type testLogOrder struct{}

func (l *testLogOrder) Handle(ctx context.Context, event Event) error {
	return nil
}

type testUpdateInventory struct{}

func (l *testUpdateInventory) Handle(ctx context.Context, event Event) error {
	return nil
}

type testLowPriorityListener struct{}

func (l *testLowPriorityListener) Handle(ctx context.Context, event Event) error {
	return nil
}

func (l *testLowPriorityListener) Priority() int { return 10 }

type testHighPriorityListener struct{}

func (l *testHighPriorityListener) Handle(ctx context.Context, event Event) error {
	return nil
}

func (l *testHighPriorityListener) Priority() int { return 100 }

type testMediumPriorityListener struct{}

func (l *testMediumPriorityListener) Handle(ctx context.Context, event Event) error {
	return nil
}

func (l *testMediumPriorityListener) Priority() int { return 50 }

type testQueueableListener struct{}

func (l *testQueueableListener) Handle(ctx context.Context, event Event) error {
	return nil
}

func (l *testQueueableListener) ShouldQueue() bool         { return true }
func (l *testQueueableListener) Queue() string             { return "emails" }
func (l *testQueueableListener) Delay() time.Duration      { return time.Second * 10 }
func (l *testQueueableListener) Tries() int                { return 3 }

type testStoppableListener struct {
	shouldStop bool
	handleFunc func(ctx context.Context, e Event) error
}

func (l *testStoppableListener) Handle(ctx context.Context, e Event) error {
	if l.handleFunc != nil {
		return l.handleFunc(ctx, e)
	}
	return nil
}

func (l *testStoppableListener) ShouldStop(ctx context.Context, e Event) bool {
	return l.shouldStop
}

// 确保Dispatcher实现了必要的接口
var _ watermill.LoggerAdapter = (*ZapLoggerAdapter)(nil)
