package event

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestNewZapLoggerAdapter 测试创建Zap适配器
func TestNewZapLoggerAdapter(t *testing.T) {
	t.Run("creates adapter with logger", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		if adapter == nil {
			t.Fatal("expected adapter to be created")
		}
		if adapter.logger == nil {
			t.Error("expected logger to be set")
		}
		if adapter.fields == nil {
			t.Error("expected fields to be initialized")
		}
	})
}

// TestZapLoggerAdapter_Info 测试Info级别日志
func TestZapLoggerAdapter_Info(t *testing.T) {
	t.Run("logs info message", func(t *testing.T) {
		buf := &bytes.Buffer{}
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(buf),
			zapcore.InfoLevel,
		)
		logger := zap.New(core)

		adapter := NewZapLoggerAdapter(logger)
		adapter.Info("test message", watermill.LogFields{"key": "value"})

		if buf.Len() == 0 {
			t.Error("expected log output")
		}
	})

	t.Run("logs info with nil fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		// Should not panic
		adapter.Info("test message", nil)
	})
}

// TestZapLoggerAdapter_Debug 测试Debug级别日志
func TestZapLoggerAdapter_Debug(t *testing.T) {
	t.Run("logs debug message", func(t *testing.T) {
		buf := &bytes.Buffer{}
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(buf),
			zapcore.DebugLevel,
		)
		logger := zap.New(core)

		adapter := NewZapLoggerAdapter(logger)
		adapter.Debug("debug message", watermill.LogFields{"key": "value"})

		if buf.Len() == 0 {
			t.Error("expected log output")
		}
	})

	t.Run("logs debug with nil fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		// Should not panic
		adapter.Debug("debug message", nil)
	})
}

// TestZapLoggerAdapter_Error 测试Error级别日志
func TestZapLoggerAdapter_Error(t *testing.T) {
	t.Run("logs error message", func(t *testing.T) {
		buf := &bytes.Buffer{}
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(buf),
			zapcore.ErrorLevel,
		)
		logger := zap.New(core)

		adapter := NewZapLoggerAdapter(logger)
		testErr := errors.New("test error")
		adapter.Error("error message", testErr, watermill.LogFields{"key": "value"})

		if buf.Len() == 0 {
			t.Error("expected log output")
		}
	})

	t.Run("logs error with nil error and fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		// Should not panic
		adapter.Error("error message", nil, nil)
	})
}

// TestZapLoggerAdapter_Trace 测试Trace级别日志
func TestZapLoggerAdapter_Trace(t *testing.T) {
	t.Run("logs trace as debug", func(t *testing.T) {
		buf := &bytes.Buffer{}
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(buf),
			zapcore.DebugLevel,
		)
		logger := zap.New(core)

		adapter := NewZapLoggerAdapter(logger)
		adapter.Trace("trace message", watermill.LogFields{"key": "value"})

		if buf.Len() == 0 {
			t.Error("expected log output")
		}
	})

	t.Run("logs trace with nil fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		// Should not panic
		adapter.Trace("trace message", nil)
	})
}

// TestZapLoggerAdapter_With 测试创建带字段的子logger
func TestZapLoggerAdapter_With(t *testing.T) {
	t.Run("creates sub-logger with fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		subAdapter := adapter.With(watermill.LogFields{"component": "dispatcher"})

		if subAdapter == nil {
			t.Fatal("expected sub-adapter to be created")
		}

		// Should be able to log
		subAdapter.Info("test message", nil)
	})

	t.Run("chains multiple With calls", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		subAdapter1 := adapter.With(watermill.LogFields{"component": "dispatcher"})
		subAdapter2 := subAdapter1.With(watermill.LogFields{"module": "queue"})

		if subAdapter2 == nil {
			t.Fatal("expected sub-adapter to be created")
		}

		subAdapter2.Info("test message", nil)
	})

	t.Run("preserves existing fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		adapter2 := adapter.With(watermill.LogFields{"key1": "value1"})
		adapter3 := adapter2.With(watermill.LogFields{"key2": "value2"})

		zapAdapter3 := adapter3.(*ZapLoggerAdapter)

		if len(zapAdapter3.fields) != 2 {
			t.Errorf("expected 2 fields, got %d", len(zapAdapter3.fields))
		}
	})

	t.Run("handles nil fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		subAdapter := adapter.With(nil)

		if subAdapter == nil {
			t.Fatal("expected sub-adapter to be created even with nil fields")
		}
	})
}

// TestZapLoggerAdapter_GetZapLogger 测试获取底层logger
func TestZapLoggerAdapter_GetZapLogger(t *testing.T) {
	t.Run("returns underlying logger", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		got := adapter.GetZapLogger()

		if got != logger {
			t.Error("expected same logger instance")
		}
	})
}

// TestZapLoggerAdapter_toZapFields 测试字段转换
func TestZapLoggerAdapter_toZapFields(t *testing.T) {
	t.Run("converts fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		fields := watermill.LogFields{
			"string": "value",
			"number": 123,
			"bool":   true,
		}

		zapFields := adapter.toZapFields(fields)

		if len(zapFields) != 3 {
			t.Errorf("expected 3 fields, got %d", len(zapFields))
		}
	})

	t.Run("handles nil fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		zapFields := adapter.toZapFields(nil)

		if len(zapFields) != 0 {
			t.Errorf("expected 0 fields, got %d", len(zapFields))
		}
	})

	t.Run("handles empty fields", func(t *testing.T) {
		logger := zap.NewNop()
		adapter := NewZapLoggerAdapter(logger)

		zapFields := adapter.toZapFields(watermill.LogFields{})

		if len(zapFields) != 0 {
			t.Errorf("expected 0 fields, got %d", len(zapFields))
		}
	})
}
