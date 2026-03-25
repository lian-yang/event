package event

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"go.uber.org/zap"
)

const (
	DefaultQueue      = "events"
	DefaultTries      = 3
	DefaultRetryDelay = time.Second * 5

	// NoRetry 不重试，失败直接标记为失败
	NoRetry = -1
)

// createJobMessage 创建队列消息
// delay: 从监听器配置获取的延迟时间
// tries: 从监听器配置获取的重试次数
func (d *Dispatcher) createJobMessage(event Event, listenerName string, delay time.Duration, tries int) (*message.Message, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal event failed: %w", err)
	}

	queue := d.getListenerQueue(listenerName, event)

	job := &Job{
		ID:           watermill.NewUUID(),
		EventName:    event.EventName(),
		EventType:    reflect.TypeOf(event).String(),
		ListenerName: listenerName,
		Payload:      payload,
		Queue:        queue,
		Delay:        delay,
		Tries:        tries,
		Attempts:     0,
		AvailableAt:  time.Now().Add(delay),
		CreatedAt:    time.Now(),
	}

	jobPayload, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("marshal job failed: %w", err)
	}

	msg := message.NewMessage(job.ID, jobPayload)
	msg.Metadata.Set("event_name", event.EventName())
	msg.Metadata.Set("listener_name", listenerName)
	msg.Metadata.Set("queue", job.Queue)

	return msg, nil
}

// getListenerQueue 获取监听器队列名
func (d *Dispatcher) getListenerQueue(listenerName string, event Event) string {
	// 优先从监听器配置获取
	if entry, ok := d.queueListeners[listenerName]; ok && entry.queue != "" {
		return entry.queue
	}

	// 从事件接口获取
	if q, ok := event.(ShouldQueue); ok {
		if queue := q.Queue(); queue != "" {
			return queue
		}
	}

	return DefaultQueue
}

// getEventRetryDelay 获取重试延迟
func (d *Dispatcher) getEventRetryDelay(event Event) time.Duration {
	if r, ok := event.(Retryable); ok {
		return r.RetryDelay()
	}
	return DefaultRetryDelay
}

// ResolveTries 解析 Tries 值
//
//	-1: 不重试，返回 -1
//	 0: 使用默认值，返回 DefaultTries
//	>0: 使用指定值
func ResolveTries(tries int) int {
	if tries == NoRetry {
		return NoRetry
	}
	if tries <= 0 {
		return DefaultTries
	}
	return tries
}

// shouldRetry 判断是否应该重试
func (d *Dispatcher) shouldRetry(job *Job) bool {
	if job.Tries == NoRetry {
		return false
	}
	return job.Attempts < job.Tries
}

// processJob 处理队列任务
func (d *Dispatcher) processJob(msg *message.Message) error {
	var job Job
	if err := json.Unmarshal(msg.Payload, &job); err != nil {
		return fmt.Errorf("unmarshal job failed: %w", err)
	}

	// 检查延迟
	if time.Now().Before(job.AvailableAt) {
		delay := time.Until(job.AvailableAt)
		d.logDebug("job delayed",
			zap.String("job_id", job.ID),
			zap.Duration("delay", delay),
		)
		time.Sleep(delay)
	}

	job.Attempts++

	// 日志
	var retriesInfo string
	if job.Tries == NoRetry {
		retriesInfo = "no-retry"
	} else {
		retriesInfo = fmt.Sprintf("%d/%d", job.Attempts, job.Tries)
	}

	d.logDebug("processing job",
		zap.String("job_id", job.ID),
		zap.String("event", job.EventName),
		zap.String("listener", job.ListenerName),
		zap.String("attempts", retriesInfo),
	)

	// 获取事件类型和监听器
	d.mu.RLock()
	eventType, exists := d.eventTypes[job.EventName]
	if !exists {
		d.mu.RUnlock()
		return fmt.Errorf("unknown event type: %s", job.EventName)
	}

	listenerEntry, ok := d.queueListeners[job.ListenerName]
	globalMiddlewares := d.globalMiddlewares
	eventMiddlewares := d.eventMiddlewares[job.EventName]
	d.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown listener: %s", job.ListenerName)
	}

	// 反序列化事件
	eventPtr := reflect.New(eventType).Interface()
	if err := json.Unmarshal(job.Payload, eventPtr); err != nil {
		return fmt.Errorf("unmarshal event failed: %w", err)
	}
	event := eventPtr.(Event)

	// 构建中间件链
	chain := NewMiddlewareChain(globalMiddlewares...)
	chain.Append(eventMiddlewares...)
	chain.Append(listenerEntry.middlewares...)

	// 执行监听器
	ctx := msg.Context()
	handler := chain.Then(func(ctx context.Context, e Event) error {
		return listenerEntry.listener.Handle(ctx, e)
	})

	if err := handler(ctx, event); err != nil {
		if d.shouldRetry(&job) {
			job.LastError = err.Error()
			job.AvailableAt = time.Now().Add(d.getRetryDelay(listenerEntry, event))

			d.logDebug("job failed, will retry",
				zap.String("job_id", job.ID),
				zap.Int("attempt", job.Attempts),
				zap.Int("max_tries", job.Tries),
				zap.Error(err),
			)
			return d.requeueJob(&job)
		}

		d.logError("job failed permanently", err,
			zap.String("job_id", job.ID),
			zap.Int("attempts", job.Attempts),
			zap.Int("tries", job.Tries),
		)
		return d.handleFailedJob(&job, err)
	}

	d.logDebug("job completed", zap.String("job_id", job.ID))
	return nil
}

// getRetryDelay 获取重试延迟（优先监听器配置）
func (d *Dispatcher) getRetryDelay(entry *queueListenerEntry, event Event) time.Duration {
	// 优先使用监听器的 delay 作为重试间隔
	if entry.delay > 0 {
		return entry.delay
	}

	// 从监听器接口获取
	if ql, ok := entry.listener.(QueueableListener); ok {
		if delay := ql.Delay(); delay > 0 {
			return delay
		}
	}

	// 从事件接口获取
	return d.getEventRetryDelay(event)
}

// requeueJob 重新入队
func (d *Dispatcher) requeueJob(job *Job) error {
	jobPayload, err := json.Marshal(job)
	if err != nil {
		return err
	}

	msg := message.NewMessage(job.ID+"-retry", jobPayload)
	msg.Metadata.Set("event_name", job.EventName)
	msg.Metadata.Set("retry", "true")

	return d.driver.Publisher().Publish(job.Queue, msg)
}

// handleFailedJob 处理失败任务
func (d *Dispatcher) handleFailedJob(job *Job, err error) error {
	if d.failedHandler != nil {
		return d.failedHandler.Handle(context.Background(), job, err)
	}
	return nil
}
