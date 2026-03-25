package event

import (
	"context"
	"testing"
	"time"
)

// TestListenerFunc 测试 ListenerFunc 类型
func TestListenerFunc(t *testing.T) {
	t.Run("implements Listener interface", func(t *testing.T) {
		var fn ListenerFunc = func(ctx context.Context, event Event) error {
			return nil
		}

		// 验证实现了 Listener 接口
		var _ Listener = fn
	})

	t.Run("calls underlying function", func(t *testing.T) {
		called := false
		fn := ListenerFunc(func(ctx context.Context, event Event) error {
			called = true
			return nil
		})

		err := fn.Handle(context.Background(), &testEvent{name: "test"})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected function to be called")
		}
	})
}

// TestMiddlewareFunc 测试 MiddlewareFunc 类型
func TestMiddlewareFunc(t *testing.T) {
	t.Run("implements Middleware interface", func(t *testing.T) {
		var fn MiddlewareFunc = func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		}

		// 验证实现了 Middleware 接口
		var _ Middleware = fn
	})

	t.Run("calls next handler", func(t *testing.T) {
		nextCalled := false
		middleware := MiddlewareFunc(func(ctx context.Context, event Event, next Handler) error {
			return next(ctx, event)
		})

		next := func(ctx context.Context, event Event) error {
			nextCalled = true
			return nil
		}

		err := middleware.Handle(context.Background(), &testEvent{name: "test"}, next)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !nextCalled {
			t.Error("expected next handler to be called")
		}
	})
}

// TestFailedJobHandlerFunc 测试 FailedJobHandlerFunc 类型
func TestFailedJobHandlerFunc(t *testing.T) {
	t.Run("implements FailedJobHandler interface", func(t *testing.T) {
		var fn FailedJobHandlerFunc = func(ctx context.Context, job *Job, err error) error {
			return nil
		}

		// 验证实现了 FailedJobHandler 接口
		var _ FailedJobHandler = fn
	})

	t.Run("calls underlying function", func(t *testing.T) {
		called := false
		handler := FailedJobHandlerFunc(func(ctx context.Context, job *Job, err error) error {
			called = true
			return nil
		})

		job := &Job{ID: "test-123"}
		testErr := error(new(testError))

		err := handler.Handle(context.Background(), job, testErr)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected function to be called")
		}
	})
}

// TestJob 测试 Job 结构体
func TestJob(t *testing.T) {
	t.Run("creates job with all fields", func(t *testing.T) {
		now := time.Now()
		job := &Job{
			ID:           "job-123",
			EventName:    "user.registered",
			EventType:    "*event.UserRegistered",
			ListenerName: "SendWelcomeEmail",
			Payload:      []byte(`{"user_id":1}`),
			Queue:        "emails",
			Delay:        time.Second * 10,
			Tries:        3,
			Attempts:     0,
			LastError:    "",
			AvailableAt:  now.Add(time.Second * 10),
			CreatedAt:    now,
		}

		if job.ID != "job-123" {
			t.Errorf("expected ID job-123, got %s", job.ID)
		}
		if job.EventName != "user.registered" {
			t.Errorf("expected event name user.registered, got %s", job.EventName)
		}
		if job.Queue != "emails" {
			t.Errorf("expected queue emails, got %s", job.Queue)
		}
		if job.Tries != 3 {
			t.Errorf("expected tries 3, got %d", job.Tries)
		}
	})
}

// ==================== 测试辅助类型 ====================

type testEvent struct {
	name string
}

func (e *testEvent) EventName() string {
	return e.name
}

type testError struct{}

func (e *testError) Error() string {
	return "test error"
}
