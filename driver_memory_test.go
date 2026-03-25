package event

import (
	"context"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.uber.org/zap/zaptest"
)

// TestNewMemoryDriver 测试创建内存驱动
func TestNewMemoryDriver(t *testing.T) {
	t.Run("creates driver with defaults", func(t *testing.T) {
		driver := NewMemoryDriver()

		if driver == nil {
			t.Fatal("expected driver to be created")
		}
		if driver.pubSub == nil {
			t.Error("expected pubSub to be initialized")
		}
	})

	t.Run("creates driver with options", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)

		driver := NewMemoryDriver(
			WithDriverLogger(wmLogger),
		)

		if driver == nil {
			t.Fatal("expected driver to be created")
		}
	})
}

// TestMemoryDriver_Publisher 测试发布者
func TestMemoryDriver_Publisher(t *testing.T) {
	driver := NewMemoryDriver()
	defer driver.Close()

	t.Run("returns publisher", func(t *testing.T) {
		pub := driver.Publisher()

		if pub == nil {
			t.Error("expected publisher to not be nil")
		}
	})
}

// TestMemoryDriver_Subscriber 测试订阅者
func TestMemoryDriver_Subscriber(t *testing.T) {
	driver := NewMemoryDriver()
	defer driver.Close()

	t.Run("returns subscriber", func(t *testing.T) {
		sub := driver.Subscriber()

		if sub == nil {
			t.Error("expected subscriber to not be nil")
		}
	})
}

// TestMemoryDriver_Close 测试关闭驱动
func TestMemoryDriver_Close(t *testing.T) {
	t.Run("closes without error", func(t *testing.T) {
		driver := NewMemoryDriver()

		err := driver.Close()

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("can be closed multiple times", func(t *testing.T) {
		driver := NewMemoryDriver()

		_ = driver.Close()
		_ = driver.Close() // Should not panic
	})
}

// TestMemoryDriver_PublishSubscribe 测试发布订阅功能
func TestMemoryDriver_PublishSubscribe(t *testing.T) {
	driver := NewMemoryDriver()
	defer driver.Close()

	t.Run("publishes and receives messages", func(t *testing.T) {
		ctx := context.Background()
		topic := "test-topic"

		// Subscribe
		messages, err := driver.Subscriber().Subscribe(ctx, topic)
		if err != nil {
			t.Fatalf("failed to subscribe: %v", err)
		}

		// Publish
		msg := message.NewMessage("test-uuid", []byte(`{"test":"data"}`))
		err = driver.Publisher().Publish(topic, msg)
		if err != nil {
			t.Fatalf("failed to publish: %v", err)
		}

		// Receive
		select {
		case receivedMsg := <-messages:
			if receivedMsg.UUID != msg.UUID {
				t.Errorf("expected UUID %s, got %s", msg.UUID, receivedMsg.UUID)
			}
		case <-time.After(time.Second):
			t.Error("expected to receive message within 1 second")
		}
	})

	t.Run("handles multiple messages", func(t *testing.T) {
		ctx := context.Background()
		topic := "multi-test"

		messages, _ := driver.Subscriber().Subscribe(ctx, topic)

		// Publish multiple messages
		for i := 0; i < 5; i++ {
			msg := message.NewMessage("test-uuid", []byte("test"))
			_ = driver.Publisher().Publish(topic, msg)
		}

		// Receive all messages
		received := 0
		timeout := time.After(time.Second * 2)

		for received < 5 {
			select {
			case <-messages:
				received++
			case <-timeout:
				t.Errorf("expected 5 messages, received %d", received)
				return
			}
		}

		if received != 5 {
			t.Errorf("expected 5 messages, got %d", received)
		}
	})
}

// TestMemoryDriver_Options 测试驱动选项
func TestMemoryDriver_Options(t *testing.T) {
	t.Run("WithDriverLogger sets logger", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		wmLogger := NewZapLoggerAdapter(logger)

		driver := NewMemoryDriver(WithDriverLogger(wmLogger))

		if driver.pubSub == nil {
			t.Error("expected pubSub to be initialized")
		}

		_ = driver.Close()
	})
}
