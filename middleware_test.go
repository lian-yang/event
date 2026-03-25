package event

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestMiddlewareChain 测试中间件链
func TestMiddlewareChain(t *testing.T) {
	t.Run("NewMiddlewareChain creates empty chain", func(t *testing.T) {
		chain := NewMiddlewareChain()

		if chain == nil {
			t.Fatal("expected chain to be created")
		}
		if len(chain.middlewares) != 0 {
			t.Errorf("expected 0 middlewares, got %d", len(chain.middlewares))
		}
	})

	t.Run("Append adds middlewares", func(t *testing.T) {
		m1 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})
		m2 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})

		chain := NewMiddlewareChain(m1).Append(m2)

		if len(chain.middlewares) != 2 {
			t.Errorf("expected 2 middlewares, got %d", len(chain.middlewares))
		}
	})

	t.Run("Prepend adds middlewares at beginning", func(t *testing.T) {
		var order []string

		m1 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			order = append(order, "m1")
			return next(ctx, event)
		})
		m2 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			order = append(order, "m2")
			return next(ctx, event)
		})

		chain := NewMiddlewareChain(m1).Prepend(m2)

		if len(chain.middlewares) != 2 {
			t.Errorf("expected 2 middlewares, got %d", len(chain.middlewares))
		}

		// 验证顺序: m2, m1
		handler := chain.Then(func(ctx context.Context, event Event) error {
			order = append(order, "handler")
			return nil
		})

		_ = handler(context.Background(), &testEvent{name: "test"})

		expected := []string{"m2", "m1", "handler"}
		for i, v := range expected {
			if i >= len(order) || order[i] != v {
				t.Errorf("expected order[%d] = %s, got %v", i, v, order)
			}
		}
	})

	t.Run("Then returns final handler when chain is empty", func(t *testing.T) {
		chain := NewMiddlewareChain()

		called := false
		finalHandler := func(ctx context.Context, event Event) error {
			called = true
			return nil
		}

		handler := chain.Then(finalHandler)
		_ = handler(context.Background(), &testEvent{name: "test"})

		if !called {
			t.Error("expected final handler to be called")
		}
	})

	t.Run("Then executes middlewares in reverse order", func(t *testing.T) {
		var order []string

		m1 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			order = append(order, "m1-before")
			err := next(ctx, event)
			order = append(order, "m1-after")
			return err
		})
		m2 := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			order = append(order, "m2-before")
			err := next(ctx, event)
			order = append(order, "m2-after")
			return err
		})

		chain := NewMiddlewareChain(m1, m2)

		handler := chain.Then(func(ctx context.Context, event Event) error {
			order = append(order, "handler")
			return nil
		})

		_ = handler(context.Background(), &testEvent{name: "test"})

		expected := []string{
			"m1-before", "m2-before", "handler", "m2-after", "m1-after",
		}

		if len(order) != len(expected) {
			t.Errorf("expected %d entries, got %d", len(expected), len(order))
		}

		for i, v := range expected {
			if i >= len(order) || order[i] != v {
				t.Errorf("expected order[%d] = %s, got %s", i, v, order[i])
			}
		}
	})
}

// TestRecoveryMiddleware 测试恢复中间件
func TestRecoveryMiddleware(t *testing.T) {
	t.Run("recovers from panic", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		middleware := NewRecoveryMiddleware(logger)

		panicErr := errors.New("test panic")
		handler := func(ctx context.Context, event Event) error {
			panic(panicErr)
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err == nil {
			t.Error("expected error from panic recovery")
		}
		expectedErr := fmt.Sprintf("panic: %v", panicErr)
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%s'", expectedErr, err.Error())
		}
	})

	t.Run("passes through normal execution", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		middleware := NewRecoveryMiddleware(logger)

		called := false
		handler := func(ctx context.Context, event Event) error {
			called = true
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected handler to be called")
		}
	})
}

// TestLoggingMiddleware 测试日志中间件
func TestLoggingMiddleware(t *testing.T) {
	t.Run("logs successful processing", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		middleware := NewLoggingMiddleware(logger)

		called := false
		handler := func(ctx context.Context, event Event) error {
			called = true
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected handler to be called")
		}
	})

	t.Run("logs failed processing", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		middleware := NewLoggingMiddleware(logger)

		expectedErr := errors.New("handler error")
		handler := func(ctx context.Context, event Event) error {
			return expectedErr
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})
}

// TestTimeoutMiddleware 测试超时中间件
func TestTimeoutMiddleware(t *testing.T) {
	t.Run("times out slow handler", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		timeout := time.Millisecond * 100
		middleware := NewTimeoutMiddleware(timeout, logger)

		handler := func(ctx context.Context, event Event) error {
			time.Sleep(time.Millisecond * 200)
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err == nil {
			t.Error("expected timeout error")
		}
	})

	t.Run("allows fast handler", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		timeout := time.Second * 5
		middleware := NewTimeoutMiddleware(timeout, logger)

		called := false
		handler := func(ctx context.Context, event Event) error {
			called = true
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected handler to be called")
		}
	})
}

// TestRetryMiddleware 测试重试中间件
func TestRetryMiddleware(t *testing.T) {
	t.Run("retries on failure", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		maxRetries := 3
		delay := time.Millisecond * 10
		middleware := NewRetryMiddleware(maxRetries, delay, logger)

		attempts := 0
		handler := func(ctx context.Context, event Event) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != nil {
			t.Errorf("expected no error after retries, got %v", err)
		}
		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("fails after max retries", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		maxRetries := 2
		delay := time.Millisecond * 10
		middleware := NewRetryMiddleware(maxRetries, delay, logger)

		attempts := 0
		expectedErr := errors.New("permanent error")
		handler := func(ctx context.Context, event Event) error {
			attempts++
			return expectedErr
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err == nil {
			t.Error("expected error after max retries")
		}
		// maxRetries=2 表示重试2次,总共执行3次 (初始1次 + 重试2次)
		if attempts != 3 {
			t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", attempts)
		}
	})

	t.Run("succeeds immediately", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		maxRetries := 3
		delay := time.Millisecond * 10
		middleware := NewRetryMiddleware(maxRetries, delay, logger)

		attempts := 0
		handler := func(ctx context.Context, event Event) error {
			attempts++
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
	})
}

// TestRateLimitMiddleware 测试限流中间件
func TestRateLimitMiddleware(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		maxConcurrent := 2
		timeout := time.Second * 5
		middleware := NewRateLimitMiddleware(maxConcurrent, timeout)

		called := false
		handler := func(ctx context.Context, event Event) error {
			called = true
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test"},
			handler,
		)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected handler to be called")
		}
	})

	t.Run("rejects requests when limit exceeded", func(t *testing.T) {
		maxConcurrent := 1
		timeout := time.Millisecond * 100
		middleware := NewRateLimitMiddleware(maxConcurrent, timeout)

		// 占用所有槽位
		blockingHandler := func(ctx context.Context, event Event) error {
			time.Sleep(time.Second * 2)
			return nil
		}

		// 启动第一个请求占用槽位
		go func() {
			_ = middleware.Handle(
				context.Background(),
				&testEvent{name: "test1"},
				blockingHandler,
			)
		}()

		// 等待确保第一个请求已经开始
		time.Sleep(time.Millisecond * 50)

		// 第二个请求应该超时
		handler := func(ctx context.Context, event Event) error {
			return nil
		}

		err := middleware.Handle(
			context.Background(),
			&testEvent{name: "test2"},
			handler,
		)

		if err == nil {
			t.Error("expected rate limit exceeded error")
		}
	})
}
