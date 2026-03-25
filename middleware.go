package event

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"go.uber.org/zap"
)

// ==================== 中间件链 ====================

// MiddlewareChain 中间件链
type MiddlewareChain struct {
	middlewares []Middleware
}

// NewMiddlewareChain 创建中间件链
func NewMiddlewareChain(middlewares ...Middleware) *MiddlewareChain {
	return &MiddlewareChain{middlewares: middlewares}
}

// Append 添加中间件
func (c *MiddlewareChain) Append(m ...Middleware) *MiddlewareChain {
	c.middlewares = append(c.middlewares, m...)
	return c
}

// Prepend 前置添加中间件
func (c *MiddlewareChain) Prepend(m ...Middleware) *MiddlewareChain {
	c.middlewares = append(m, c.middlewares...)
	return c
}

// Then 执行中间件链
func (c *MiddlewareChain) Then(final Handler) Handler {
	if len(c.middlewares) == 0 {
		return final
	}

	h := final
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		middleware := c.middlewares[i]
		next := h
		h = func(ctx context.Context, event Event) error {
			return middleware.Handle(ctx, event, next)
		}
	}

	return h
}

// ==================== 内置中间件 ====================

// RecoveryMiddleware 恢复中间件
type RecoveryMiddleware struct {
	logger *zap.Logger
}

func NewRecoveryMiddleware(logger *zap.Logger) *RecoveryMiddleware {
	return &RecoveryMiddleware{logger: logger}
}

func (m *RecoveryMiddleware) Handle(ctx context.Context, event Event, next Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			if m.logger != nil {
				m.logger.Error("event panic recovered",
					zap.String("event", event.EventName()),
					zap.Any("panic", r),
					zap.ByteString("stack", stack),
				)
			}
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return next(ctx, event)
}

// LoggingMiddleware 日志中间件
type LoggingMiddleware struct {
	logger *zap.Logger
}

func NewLoggingMiddleware(logger *zap.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{logger: logger}
}

func (m *LoggingMiddleware) Handle(ctx context.Context, event Event, next Handler) error {
	start := time.Now()

	if m.logger != nil {
		m.logger.Debug("event processing started",
			zap.String("event", event.EventName()),
		)
	}

	err := next(ctx, event)
	duration := time.Since(start)

	if m.logger != nil {
		if err != nil {
			m.logger.Error("event processing failed",
				zap.String("event", event.EventName()),
				zap.Duration("duration", duration),
				zap.Error(err),
			)
		} else {
			m.logger.Debug("event processing completed",
				zap.String("event", event.EventName()),
				zap.Duration("duration", duration),
			)
		}
	}

	return err
}

// TimeoutMiddleware 超时中间件
type TimeoutMiddleware struct {
	timeout time.Duration
	logger  *zap.Logger
}

func NewTimeoutMiddleware(timeout time.Duration, logger *zap.Logger) *TimeoutMiddleware {
	return &TimeoutMiddleware{timeout: timeout, logger: logger}
}

func (m *TimeoutMiddleware) Handle(ctx context.Context, event Event, next Handler) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- next(ctx, event)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if m.logger != nil {
			m.logger.Warn("event processing timeout",
				zap.String("event", event.EventName()),
				zap.Duration("timeout", m.timeout),
			)
		}
		return fmt.Errorf("event timeout: %s", event.EventName())
	}
}

// RetryMiddleware 重试中间件
type RetryMiddleware struct {
	maxRetries int
	delay      time.Duration
	backoff    float64
	logger     *zap.Logger
}

func NewRetryMiddleware(maxRetries int, delay time.Duration, logger *zap.Logger) *RetryMiddleware {
	return &RetryMiddleware{
		maxRetries: maxRetries,
		delay:      delay,
		backoff:    2.0,
		logger:     logger,
	}
}

func (m *RetryMiddleware) Handle(ctx context.Context, event Event, next Handler) error {
	var lastErr error
	delay := m.delay

	for i := 0; i <= m.maxRetries; i++ {
		if i > 0 {
			if m.logger != nil {
				m.logger.Debug("retrying event",
					zap.String("event", event.EventName()),
					zap.Int("attempt", i),
					zap.Int("max_retries", m.maxRetries),
				)
			}
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * m.backoff)
		}

		lastErr = next(ctx, event)
		if lastErr == nil {
			return nil
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// RateLimitMiddleware 限流中间件
type RateLimitMiddleware struct {
	limiter chan struct{}
	timeout time.Duration
}

func NewRateLimitMiddleware(maxConcurrent int, timeout time.Duration) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: make(chan struct{}, maxConcurrent),
		timeout: timeout,
	}
}

func (m *RateLimitMiddleware) Handle(ctx context.Context, event Event, next Handler) error {
	select {
	case m.limiter <- struct{}{}:
		defer func() { <-m.limiter }()
		return next(ctx, event)
	case <-time.After(m.timeout):
		return fmt.Errorf("rate limit exceeded: %s", event.EventName())
	case <-ctx.Done():
		return ctx.Err()
	}
}
