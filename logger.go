package event

import (
	"github.com/ThreeDotsLabs/watermill"
	"go.uber.org/zap"
)

// ZapLoggerAdapter 实现 watermill.LoggerAdapter 接口
type ZapLoggerAdapter struct {
	logger *zap.Logger
	fields watermill.LogFields
}

// NewZapLoggerAdapter 创建 Zap 日志适配器
func NewZapLoggerAdapter(logger *zap.Logger) *ZapLoggerAdapter {
	return &ZapLoggerAdapter{
		logger: logger,
		fields: make(watermill.LogFields),
	}
}

// Error 错误日志
func (z *ZapLoggerAdapter) Error(msg string, err error, fields watermill.LogFields) {
	zapFields := z.toZapFields(fields)
	if err != nil {
		zapFields = append(zapFields, zap.Error(err))
	}
	z.logger.Error(msg, zapFields...)
}

// Info 信息日志
func (z *ZapLoggerAdapter) Info(msg string, fields watermill.LogFields) {
	z.logger.Info(msg, z.toZapFields(fields)...)
}

// Debug 调试日志
func (z *ZapLoggerAdapter) Debug(msg string, fields watermill.LogFields) {
	z.logger.Debug(msg, z.toZapFields(fields)...)
}

// Trace 追踪日志
func (z *ZapLoggerAdapter) Trace(msg string, fields watermill.LogFields) {
	z.logger.Debug(msg, z.toZapFields(fields)...)
}

// With 创建带有预设字段的新 Logger
func (z *ZapLoggerAdapter) With(fields watermill.LogFields) watermill.LoggerAdapter {
	newFields := make(watermill.LogFields)
	for k, v := range z.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &ZapLoggerAdapter{
		logger: z.logger.With(z.toZapFields(fields)...),
		fields: newFields,
	}
}

// toZapFields 转换字段
func (z *ZapLoggerAdapter) toZapFields(fields watermill.LogFields) []zap.Field {
	if fields == nil {
		return nil
	}

	zapFields := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		zapFields = append(zapFields, zap.Any(key, value))
	}
	return zapFields
}

// GetZapLogger 获取底层 Zap Logger
func (z *ZapLoggerAdapter) GetZapLogger() *zap.Logger {
	return z.logger
}
