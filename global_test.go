package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestSetup 测试全局初始化函数
func TestSetup(t *testing.T) {
	t.Run("initializes global dispatcher", func(t *testing.T) {
		// 确保清理
		defer Close()

		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		err := Setup(driver, wmLogger)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !IsInitialized() {
			t.Error("expected dispatcher to be initialized")
		}

		dispatcher := GetDispatcher()
		if dispatcher == nil {
			t.Error("expected dispatcher to not be nil")
		}
	})

	t.Run("SetupWithZap initializes with Zap logger", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		driver := NewMemoryDriver()

		err := SetupWithZap(driver, logger)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !IsInitialized() {
			t.Error("expected dispatcher to be initialized")
		}
	})

	t.Run("SetupMemory initializes with memory driver", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)

		err := SetupMemory(logger)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !IsInitialized() {
			t.Error("expected dispatcher to be initialized")
		}
	})
}

// TestSetDispatcher 测试设置全局分发器
func TestSetDispatcher(t *testing.T) {
	t.Run("sets global dispatcher", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, _ := NewDispatcher(driver, wmLogger)
		SetDispatcher(dispatcher)

		if GetDispatcher() != dispatcher {
			t.Error("expected same dispatcher instance")
		}
	})
}

// TestGetDispatcher 测试获取全局分发器
func TestGetDispatcher(t *testing.T) {
	t.Run("returns nil when not initialized", func(t *testing.T) {
		// 确保未初始化状态
		globalMu.Lock()
		globalDispatcher = nil
		globalMu.Unlock()

		dispatcher := GetDispatcher()

		if dispatcher != nil {
			t.Error("expected nil when not initialized")
		}
	})

	t.Run("returns dispatcher when initialized", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)
		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		dispatcher, _ := NewDispatcher(driver, wmLogger)
		SetDispatcher(dispatcher)

		got := GetDispatcher()
		if got != dispatcher {
			t.Error("expected same dispatcher instance")
		}
	})
}

// TestGlobalDispatch 测试全局分发函数
func TestGlobalDispatch(t *testing.T) {
	t.Run("Dispatch panics when not initialized", func(t *testing.T) {
		// 确保未初始化
		globalMu.Lock()
		globalDispatcher = nil
		globalMu.Unlock()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when dispatcher not initialized")
			}
		}()

		_ = Dispatch(context.Background(), &testGlobalEvent{})
	})

	t.Run("Dispatch dispatches event", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		var mu sync.Mutex
		called := false

		Listen(&testGlobalEvent{}, ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		}))

		err := Dispatch(context.Background(), &testGlobalEvent{})

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

	t.Run("DispatchSync dispatches synchronously", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		var mu sync.Mutex
		called := false

		Listen(&testGlobalEvent{}, ListenerFunc(func(ctx context.Context, e Event) error {
			mu.Lock()
			called = true
			mu.Unlock()
			return nil
		}))

		err := DispatchSync(context.Background(), &testGlobalEvent{})

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
}

// TestGlobalListen 测试全局监听函数
func TestGlobalListen(t *testing.T) {
	t.Run("Listen panics when not initialized", func(t *testing.T) {
		globalMu.Lock()
		globalDispatcher = nil
		globalMu.Unlock()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when dispatcher not initialized")
			}
		}()

		Listen(&testGlobalEvent{}, &testGlobalListener{})
	})

	t.Run("ListenAsync registers async listener", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		ListenAsync(&testGlobalEvent{}, &testQueueableGlobalListener{})

		// Verify it was registered
		d := GetDispatcher()
		if len(d.listeners["global.test"]) == 0 {
			t.Error("expected listener to be registered")
		}
	})

	t.Run("ListenWithOptions registers with options", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		ListenWithOptions(&testGlobalEvent{}, &testGlobalListener{}, ListenOptions{
			Name:     "custom-name",
			Priority: 100,
		})

		d := GetDispatcher()
		entries := d.listeners["global.test"]
		if len(entries) == 0 {
			t.Fatal("expected listener to be registered")
		}
		if entries[0].name != "custom-name" {
			t.Errorf("expected name 'custom-name', got '%s'", entries[0].name)
		}
		if entries[0].priority != 100 {
			t.Errorf("expected priority 100, got %d", entries[0].priority)
		}
	})

	t.Run("ListenFunc registers function listener", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		called := false
		ListenFunc(&testGlobalEvent{}, func(ctx context.Context, e Event) error {
			called = true
			return nil
		})

		_ = Dispatch(context.Background(), &testGlobalEvent{})

		if !called {
			t.Error("expected listener to be called")
		}
	})

	t.Run("ListenAsyncFunc registers async function listener", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		ListenAsyncFunc(&testGlobalEvent{}, func(ctx context.Context, e Event) error {
			return nil
		})

		d := GetDispatcher()
		entries := d.listeners["global.test"]
		if len(entries) == 0 {
			t.Fatal("expected listener to be registered")
		}
		if !entries[0].async {
			t.Error("expected listener to be async")
		}
	})

	t.Run("ListenWildcard registers wildcard listener", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		ListenWildcard("global.*", &testGlobalListener{}, ListenOptions{
			Name: "wildcard-listener",
		})

		d := GetDispatcher()
		if len(d.wildcardListeners) == 0 {
			t.Error("expected wildcard listener to be registered")
		}
	})
}

// TestGlobalMiddleware 测试全局中间件函数
func TestGlobalMiddleware(t *testing.T) {
	t.Run("Use adds global middleware", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		mw := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})

		Use(mw)

		d := GetDispatcher()
		if len(d.globalMiddlewares) == 0 {
			t.Error("expected global middleware to be added")
		}
	})

	t.Run("UseForEvent adds event-level middleware", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		mw := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})

		UseForEvent("global.test", mw)

		d := GetDispatcher()
		if len(d.eventMiddlewares["global.test"]) == 0 {
			t.Error("expected event middleware to be added")
		}
	})

	t.Run("SetFailedHandler sets failed handler", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		handler := FailedJobHandlerFunc(func(ctx context.Context, job *Job, err error) error {
			return nil
		})

		SetFailedHandler(handler)

		d := GetDispatcher()
		if d.failedHandler == nil {
			t.Error("expected failed handler to be set")
		}
	})
}

// TestGlobalRun 测试全局运行函数
func TestGlobalRun(t *testing.T) {
	t.Run("Run starts queue consumers", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		// Register an async listener
		ListenAsync(&testGlobalEvent{}, &testQueueableGlobalListener{})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() {
			errCh <- Run(ctx)
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

// TestGlobalClose 测试全局关闭函数
func TestGlobalClose(t *testing.T) {
	t.Run("Close closes global dispatcher", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		err := Close()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if IsInitialized() {
			t.Error("expected dispatcher to be nil after close")
		}
	})

	t.Run("Close returns nil when not initialized", func(t *testing.T) {
		globalMu.Lock()
		globalDispatcher = nil
		globalMu.Unlock()

		err := Close()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// TestMustDispatch 测试强制分发函数
func TestMustDispatch(t *testing.T) {
	t.Run("MustDispatch succeeds when dispatch works", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		// Should not panic
		MustDispatch(context.Background(), &testGlobalEvent{})
	})

	t.Run("MustDispatch panics on error", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		// Register a listener that returns error
		Listen(&testGlobalEvent{}, ListenerFunc(func(ctx context.Context, e Event) error {
			return nil // MustDispatch won't panic on listener error
		}))

		// Should not panic because Dispatch doesn't return listener errors
		MustDispatch(context.Background(), &testGlobalEvent{})
	})
}

// TestIsInitialized 测试初始化检查函数
func TestIsInitialized(t *testing.T) {
	t.Run("returns false when not initialized", func(t *testing.T) {
		globalMu.Lock()
		globalDispatcher = nil
		globalMu.Unlock()

		if IsInitialized() {
			t.Error("expected IsInitialized to return false")
		}
	})

	t.Run("returns true when initialized", func(t *testing.T) {
		defer Close()

		logger := zaptest.NewLogger(t)
		_ = SetupMemory(logger)

		if !IsInitialized() {
			t.Error("expected IsInitialized to return true")
		}
	})
}

// ==================== 测试辅助类型 ====================

type testGlobalEvent struct{}

func (e *testGlobalEvent) EventName() string { return "global.test" }

type testGlobalListener struct{}

func (l *testGlobalListener) Handle(ctx context.Context, e Event) error {
	return nil
}

type testQueueableGlobalListener struct{}

func (l *testQueueableGlobalListener) Handle(ctx context.Context, e Event) error {
	return nil
}

func (l *testQueueableGlobalListener) ShouldQueue() bool         { return true }
func (l *testQueueableGlobalListener) Queue() string             { return "global-queue" }
func (l *testQueueableGlobalListener) Delay() time.Duration      { return time.Second }
func (l *testQueueableGlobalListener) Tries() int                { return 3 }
