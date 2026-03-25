package event

import (
	"context"
	"testing"
	"time"
)

// TestNewListener 测试创建监听器构建器
func TestNewListener(t *testing.T) {
	t.Run("creates builder with default values", func(t *testing.T) {
		handler := func(ctx context.Context, event Event) error {
			return nil
		}

		builder := NewListener(handler)

		if builder == nil {
			t.Fatal("expected builder to be created")
		}
		if builder.handler == nil {
			t.Error("expected handler to be set")
		}
		if builder.tries != 0 {
			t.Errorf("expected default tries 0, got %d", builder.tries)
		}
		if builder.queue != DefaultQueue {
			t.Errorf("expected default queue %s, got %s", DefaultQueue, builder.queue)
		}
	})
}

// TestListenerBuilder 测试监听器构建器的各种配置方法
func TestListenerBuilder(t *testing.T) {
	handler := func(ctx context.Context, event Event) error {
		return nil
	}

	t.Run("Priority sets priority", func(t *testing.T) {
		builder := NewListener(handler).Priority(100)

		if builder.priority != 100 {
			t.Errorf("expected priority 100, got %d", builder.priority)
		}
	})

	t.Run("OnQueue sets queue and enables async", func(t *testing.T) {
		builder := NewListener(handler).OnQueue("emails")

		if builder.queue != "emails" {
			t.Errorf("expected queue emails, got %s", builder.queue)
		}
		if !builder.shouldQueue {
			t.Error("expected shouldQueue to be true")
		}
	})

	t.Run("Async enables async processing", func(t *testing.T) {
		builder := NewListener(handler).Async()

		if !builder.shouldQueue {
			t.Error("expected shouldQueue to be true")
		}
	})

	t.Run("Delay sets delay duration", func(t *testing.T) {
		delay := time.Second * 10
		builder := NewListener(handler).Delay(delay)

		if builder.delay != delay {
			t.Errorf("expected delay %v, got %v", delay, builder.delay)
		}
	})

	t.Run("Tries sets retry count", func(t *testing.T) {
		builder := NewListener(handler).Tries(5)

		if builder.tries != 5 {
			t.Errorf("expected tries 5, got %d", builder.tries)
		}
	})

	t.Run("NoRetry sets tries to NoRetry constant", func(t *testing.T) {
		builder := NewListener(handler).NoRetry()

		if builder.tries != NoRetry {
			t.Errorf("expected tries %d, got %d", NoRetry, builder.tries)
		}
	})

	t.Run("Middleware appends middlewares", func(t *testing.T) {
		middleware1 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})
		middleware2 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})

		builder := NewListener(handler).Middleware(middleware1, middleware2)

		if len(builder.middlewares) != 2 {
			t.Errorf("expected 2 middlewares, got %d", len(builder.middlewares))
		}
	})

	t.Run("StopWhen sets stop function", func(t *testing.T) {
		stopFunc := func(ctx context.Context, event Event) bool {
			return true
		}

		builder := NewListener(handler).StopWhen(stopFunc)

		if builder.stopFunc == nil {
			t.Error("expected stopFunc to be set")
		}
	})

	t.Run("builder supports method chaining", func(t *testing.T) {
		builder := NewListener(handler).
			Priority(50).
			OnQueue("notifications").
			Delay(time.Minute).
			Tries(3).
			NoRetry()

		// NoRetry 会覆盖之前的 Tries(3)
		if builder.priority != 50 {
			t.Errorf("expected priority 50, got %d", builder.priority)
		}
		if builder.queue != "notifications" {
			t.Errorf("expected queue notifications, got %s", builder.queue)
		}
		if builder.delay != time.Minute {
			t.Errorf("expected delay 1m, got %v", builder.delay)
		}
		if builder.tries != NoRetry {
			t.Errorf("expected tries %d, got %d", NoRetry, builder.tries)
		}
	})
}

// TestBuiltListener 测试构建完成的监听器
func TestBuiltListener(t *testing.T) {
	t.Run("Build creates BuiltListener with all configurations", func(t *testing.T) {
		called := false
		handler := func(ctx context.Context, event Event) error {
			called = true
			return nil
		}

		middleware := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})

		stopFunc := func(ctx context.Context, event Event) bool {
			return false
		}

		listener := NewListener(handler).
			Priority(100).
			OnQueue("test-queue").
			Delay(time.Second * 5).
			Tries(3).
			Middleware(middleware).
			StopWhen(stopFunc).
			Build()

		// 验证 Handle 方法
		event := &testEvent{name: "test"}
		err := listener.Handle(context.Background(), event)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected handler to be called")
		}

		// 验证各个接口方法
		if listener.Priority() != 100 {
			t.Errorf("expected priority 100, got %d", listener.Priority())
		}
		if !listener.ShouldQueue() {
			t.Error("expected ShouldQueue to be true")
		}
		if listener.Queue() != "test-queue" {
			t.Errorf("expected queue test-queue, got %s", listener.Queue())
		}
		if listener.Delay() != time.Second*5 {
			t.Errorf("expected delay 5s, got %v", listener.Delay())
		}
		if listener.Tries() != 3 {
			t.Errorf("expected tries 3, got %d", listener.Tries())
		}
		if len(listener.Middleware()) != 1 {
			t.Errorf("expected 1 middleware, got %d", len(listener.Middleware()))
		}
		if listener.ShouldStop(context.Background(), event) {
			t.Error("expected ShouldStop to return false")
		}
	})

	t.Run("ShouldStop returns false when stopFunc is nil", func(t *testing.T) {
		listener := NewListener(func(ctx context.Context, event Event) error {
			return nil
		}).Build()

		event := &testEvent{name: "test"}
		if listener.ShouldStop(context.Background(), event) {
			t.Error("expected ShouldStop to return false when stopFunc is nil")
		}
	})

	t.Run("ShouldStop calls stopFunc when set", func(t *testing.T) {
		stopCalled := false
		listener := NewListener(func(ctx context.Context, event Event) error {
			return nil
		}).StopWhen(func(ctx context.Context, event Event) bool {
			stopCalled = true
			return true
		}).Build()

		event := &testEvent{name: "test"}
		shouldStop := listener.ShouldStop(context.Background(), event)

		if !stopCalled {
			t.Error("expected stopFunc to be called")
		}
		if !shouldStop {
			t.Error("expected ShouldStop to return true")
		}
	})
}
