package event

import (
	"testing"
	"time"
)

// TestResolveTries 测试 ResolveTries 辅助函数
func TestResolveTries(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "NoRetry constant returns NoRetry",
			input:    NoRetry,
			expected: NoRetry,
		},
		{
			name:     "zero returns DefaultTries",
			input:    0,
			expected: DefaultTries,
		},
		{
			name:     "negative (except NoRetry) returns DefaultTries",
			input:    -5,
			expected: DefaultTries,
		},
		{
			name:     "positive value returns as-is",
			input:    5,
			expected: 5,
		},
		{
			name:     "large positive value",
			input:    100,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveTries(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveTries(%d) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestConstants 测试常量定义
func TestConstants(t *testing.T) {
	t.Run("DefaultQueue is set correctly", func(t *testing.T) {
		if DefaultQueue != "events" {
			t.Errorf("expected DefaultQueue 'events', got '%s'", DefaultQueue)
		}
	})

	t.Run("DefaultTries is set correctly", func(t *testing.T) {
		if DefaultTries != 3 {
			t.Errorf("expected DefaultTries 3, got %d", DefaultTries)
		}
	})

	t.Run("DefaultRetryDelay is set correctly", func(t *testing.T) {
		if DefaultRetryDelay != time.Second*5 {
			t.Errorf("expected DefaultRetryDelay 5s, got %v", DefaultRetryDelay)
		}
	})

	t.Run("NoRetry constant is -1", func(t *testing.T) {
		if NoRetry != -1 {
			t.Errorf("expected NoRetry -1, got %d", NoRetry)
		}
	})
}
