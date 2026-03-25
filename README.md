# Go Event System

[![Go Reference](https://pkg.go.dev/badge/github.com/lian-yang/event.svg)](https://pkg.go.dev/github.com/lian-yang/event)
[![Go Report Card](https://goreportcard.com/badge/github.com/lian-yang/event)](https://goreportcard.com/report/github.com/lian-yang/event)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

一个生产级的 Go 事件系统，基于 [Watermill](https://github.com/ThreeDotsLabs/watermill) 实现，提供类似 [Laravel](https://laravel.com/docs/events) 的事件/监听器模式。

## ✨ 特性

- 🚀 **多驱动支持**: Memory、Redis、MySQL、PostgreSQL
- ⏱️ **队列功能**: Delay（延迟）、Tries（重试）、Queue（队列名）
- 🎯 **监听器功能**: Priority（优先级）、Stoppable（停止传播）
- 🔍 **通配符监听**: 支持 `*`、`user.*`、`*.created` 模式
- 🧅 **三层中间件**: 全局、事件级、监听器级
- 💉 **Wire 集成**: 完整的依赖注入支持
- 📊 **Zap 日志**: 生产级日志适配器
- 🔄 **连接复用**: 支持复用现有 Redis/数据库连接
- 🛡️ **内置中间件**: Recovery、Logging、Timeout、Retry、RateLimit
- 🔧 **Builder 模式**: 流畅的监听器配置

## 📦 安装

```bash
go get github.com/lian-yang/event@v0.1.0
```

## 🚀 快速开始

### 1. 定义事件

```go
package events

import "time"

// UserRegistered 用户注册事件
type UserRegistered struct {
    UserID    int64     `json:"user_id"`
    Email     string    `json:"email"`
    Username  string    `json:"username"`
    CreatedAt time.Time `json:"created_at"`
}

func (e *UserRegistered) EventName() string {
    return "user.registered"
}

// OrderPaid 订单支付事件（带队列配置）
type OrderPaid struct {
    OrderID int64   `json:"order_id"`
    Amount  float64 `json:"amount"`
}

func (e *OrderPaid) EventName() string { return "order.paid" }
func (e *OrderPaid) Queue() string     { return "orders" }              // 指定队列
func (e *OrderPaid) Tries() int        { return 5 }                     // 重试次数
func (e *OrderPaid) RetryDelay() time.Duration { return time.Minute }  // 重试间隔
```

### 2. 定义监听器

```go
package listeners

import (
    "context"
    "fmt"
    "log"

    "github.com/lian-yang/event"
)

// SendWelcomeEmail 发送欢迎邮件
type SendWelcomeEmail struct{}

func (l *SendWelcomeEmail) Handle(ctx context.Context, e event.Event) error {
    evt := e.(*events.UserRegistered)
    log.Printf("📧 发送欢迎邮件: %s", evt.Email)
    return nil
}

// 实现 QueueableListener 接口，自动放入队列
func (l *SendWelcomeEmail) ShouldQueue() bool     { return true }
func (l *SendWelcomeEmail) Queue() string         { return "emails" }
func (l *SendWelcomeEmail) Delay() time.Duration  { return time.Second * 10 }
func (l *SendWelcomeEmail) Tries() int            { return 3 }

// ValidateUser 验证用户（带优先级和停止传播）
type ValidateUser struct{}

func (l *ValidateUser) Handle(ctx context.Context, e event.Event) error {
    evt := e.(*events.UserRegistered)
    if evt.Email == "spam@test.com" {
        return fmt.Errorf("spam detected")
    }
    return nil
}

// 高优先级，先执行
func (l *ValidateUser) Priority() int { return 100 }

// 如果验证失败，停止后续监听器
func (l *ValidateUser) ShouldStop(ctx context.Context, e event.Event) bool {
    evt := e.(*events.UserRegistered)
    return evt.Email == "spam@test.com"
}
```

### 3. 使用构建器创建监听器

```go
// 使用 Builder 模式创建监听器
listener := event.NewListener(func(ctx context.Context, e event.Event) error {
    // 处理逻辑
    return nil
}).
    Priority(50).
    OnQueue("notifications").
    Delay(time.Minute).
    Tries(3).
    NoRetry().                    // 不重试（会覆盖 Tries(3)）
    Middleware(customMiddleware).
    StopWhen(func(ctx context.Context, e event.Event) bool {
        return false
    }).
    Build()
```

### 4. 注册事件和监听器

```go
package main

import (
    "context"
    "log"
    "time"

    "go.uber.org/zap"
    "github.com/lian-yang/event"
)

func main() {
    // 创建 Logger
    logger, _ := zap.NewProduction()
    wmLogger := event.NewZapLoggerAdapter(logger)

    // 创建驱动（使用内存驱动作为示例）
    driver := event.NewMemoryDriver(event.WithDriverLogger(wmLogger))

    // 创建分发器
    dispatcher, _ := event.NewDispatcher(driver, wmLogger)
    defer dispatcher.Close()

    // 注册全局中间件
    dispatcher.Use(
        event.NewRecoveryMiddleware(logger),
        event.NewLoggingMiddleware(logger),
    )

    // 注册事件级中间件
    dispatcher.UseForEvent("payment.*",
        event.NewTimeoutMiddleware(time.Second*30, logger),
    )

    // 注册监听器

    // 方式 1: 同步监听器
    dispatcher.Listen(&events.UserRegistered{}, &listeners.ValidateUser{})

    // 方式 2: 异步监听器（放入队列）
    dispatcher.ListenAsync(&events.UserRegistered{}, &listeners.SendWelcomeEmail{})

    // 方式 3: 带选项的监听器
    dispatcher.ListenWithOptions(&events.UserRegistered{}, &listeners.CreateProfile{}, event.ListenOptions{
        Name:     "create_profile",
        Priority: 50,
        Async:    true,
        Queue:    "profiles",
        Tries:    3,
        Middlewares: []event.Middleware{
            event.NewRetryMiddleware(2, time.Second, logger),
        },
    })

    // 方式 4: 函数监听器
    dispatcher.ListenFunc(&events.UserRegistered{}, func(ctx context.Context, e event.Event) error {
        evt := e.(*events.UserRegistered)
        log.Printf("用户注册: %s", evt.Username)
        return nil
    })

    // 方式 5: 通配符监听器
    dispatcher.ListenWildcard("user.*", &listeners.AuditLogger{}, event.ListenOptions{
        Name: "audit_user_events",
    })

    dispatcher.ListenWildcard("*", &listeners.MetricsCollector{}, event.ListenOptions{
        Name:     "metrics_all",
        Priority: -100, // 最低优先级
    })

    // 启动队列消费者
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go func() {
        if err := dispatcher.Run(ctx); err != nil {
            log.Printf("dispatcher error: %v", err)
        }
    }()

    // 触发事件
    dispatcher.Dispatch(context.Background(), &events.UserRegistered{
        UserID:    1,
        Email:     "user@example.com",
        Username:  "john",
        CreatedAt: time.Now(),
    })

    // 等待处理完成
    time.Sleep(time.Second)
}
```

## 🔌 使用全局函数（更简单）

```go
package main

import (
    "context"
    "time"

    "go.uber.org/zap"
    "github.com/lian-yang/event"
)

func init() {
    logger, _ := zap.NewProduction()

    // 初始化全局分发器
    event.SetupMemory(logger)

    // 注册监听器
    event.Listen(&UserRegistered{}, &SendWelcomeEmail{})
    event.ListenAsync(&OrderPaid{}, &UpdateInventory{})

    // 注册中间件
    event.Use(
        event.NewRecoveryMiddleware(logger),
        event.NewLoggingMiddleware(logger),
    )
}

func main() {
    // 直接分发事件
    event.Dispatch(context.Background(), &UserRegistered{
        UserID:   1,
        Email:    "user@example.com",
        Username: "john",
    })

    // 或使用链式调用
    event.MustDispatch(context.Background(), &OrderPaid{
        OrderID: 123,
        Amount:  99.99,
    })
}
```

## 💉 Wire 依赖注入

### wire.go

```go
//go:build wireinject

package main

import (
    "database/sql"

    "github.com/google/wire"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
    "github.com/lian-yang/event"
)

// 使用内存驱动
func InitializeDispatcher(logger *zap.Logger) (*event.Dispatcher, error) {
    wire.Build(event.MemorySet)
    return nil, nil
}

// 使用 Redis 驱动（从现有客户端）
func InitializeDispatcherWithRedis(logger *zap.Logger, client *redis.Client) (*event.Dispatcher, error) {
    wire.Build(event.RedisSet)
    return nil, nil
}

// 使用 MySQL 驱动（从现有数据库连接）
func InitializeDispatcherWithMySQL(logger *zap.Logger, db *sql.DB) (*event.Dispatcher, error) {
    wire.Build(event.MySQLSet)
    return nil, nil
}
```

### 使用

```go
func main() {
    logger, _ := zap.NewProduction()

    // 方式 1: 内存驱动
    dispatcher, _ := InitializeDispatcher(logger)

    // 方式 2: 复用现有 Redis 客户端
    redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    dispatcher, _ := InitializeDispatcherWithRedis(logger, redisClient)

    // 方式 3: 复用现有数据库连接
    db, _ := sql.Open("mysql", "...")
    dispatcher, _ := InitializeDispatcherWithMySQL(logger, db)
}
```

## 🧅 中间件

### 内置中间件

```go
// 恢复 panic
event.NewRecoveryMiddleware(logger)

// 日志记录
event.NewLoggingMiddleware(logger)

// 超时控制
event.NewTimeoutMiddleware(time.Second*30, logger)

// 自动重试
event.NewRetryMiddleware(3, time.Second, logger)

// 并发限流
event.NewRateLimitMiddleware(10, time.Second*5)
```

### 自定义中间件

```go
// 函数方式
customMiddleware := event.MiddlewareFunc(func(ctx context.Context, e event.Event, next event.Handler) error {
    // 前置处理
    log.Printf("Before: %s", e.EventName())

    err := next(ctx, e)

    // 后置处理
    log.Printf("After: %s", e.EventName())

    return err
})

// 结构体方式
type AuthMiddleware struct {
    validator TokenValidator
}

func (m *AuthMiddleware) Handle(ctx context.Context, e event.Event, next event.Handler) error {
    if !m.validator.Validate(ctx) {
        return errors.New("unauthorized")
    }
    return next(ctx, e)
}
```

## 🔍 通配符模式

| 模式 | 匹配示例 |
|------|----------|
| `*` | 匹配所有事件 |
| `user.*` | user.registered, user.updated, user.deleted |
| `*.created` | user.created, order.created, product.created |
| `order.*.completed` | order.payment.completed, order.shipping.completed |

```go
// 监听所有用户相关事件
dispatcher.ListenWildcard("user.*", &AuditLogger{})

// 监听所有创建事件
dispatcher.ListenWildcard("*.created", &MetricsCollector{})

// 监听所有事件
dispatcher.ListenWildcard("*", &GlobalLogger{}, event.ListenOptions{
    Priority: -100, // 最低优先级，最后执行
})
```

## ⚙️ 驱动配置

### Redis

```go
// 从配置创建
driver, _ := event.NewRedisDriver(event.RedisConfig{
    Addr:          "localhost:6379",
    Password:      "",
    DB:            0,
    ConsumerGroup: "my_app",
    Consumer:      "worker_1",
})

// 从现有客户端创建
driver, _ := event.NewRedisDriverFromClient(existingClient,
    event.WithConsumerGroup("my_app"),
    event.WithConsumer("worker_1"),
)

// 支持 Redis 集群
driver, _ := event.NewRedisDriverFromUniversalClient(clusterClient)
```

### MySQL

```go
// 从配置创建
driver, _ := event.NewMySQLDriver(event.MySQLConfig{
    DSN: "user:password@tcp(localhost:3306)/events?parseTime=true",
})

// 从现有连接创建
driver, _ := event.NewMySQLDriverFromDB(existingDB)
```

### PostgreSQL

```go
// 从配置创建
driver, _ := event.NewPostgresDriver(event.PostgresConfig{
    DSN: "postgres://user:password@localhost:5432/events?sslmode=disable",
})

// 从现有连接创建
driver, _ := event.NewPostgresDriverFromDB(existingDB)
```

## 🎯 配置优先级

### Delay 延迟配置

| 优先级 | 来源 | 说明 |
|-------|------|------|
| 1 (最高) | `ListenOptions.Delay` | 注册时指定 |
| 2 | `QueueableListener.Delay()` | 监听器接口 |
| 3 | `Delayable.Delay()` | 事件接口 |
| 4 (最低) | 0 | 默认无延迟 |

### Tries 重试配置

| 优先级 | 来源 | 说明 |
|-------|------|------|
| 1 (最高) | `ListenOptions.Tries` | 注册时指定 |
| 2 | `QueueableListener.Tries()` | 监听器接口 |
| 3 | `Retryable.Tries()` | 事件接口 |
| 4 (最低) | DefaultTries (3) | 默认值 |

### Tries 值说明

| 值 | 常量 | 说明 |
|----|------|------|
| -1 | `event.NoRetry` | 不重试，失败直接标记失败 |
| 0 | - | 使用默认值（DefaultTries = 3） |
| > 0 | - | 指定最大重试次数 |

### 示例

```go
// 监听器配置: Delay=10s, Tries=5
type MyListener struct{}
func (l *MyListener) Delay() time.Duration { return time.Second * 10 }
func (l *MyListener) Tries() int           { return 5 }

// ListenOptions 覆盖: Delay=5s, Tries=3
event.ListenAsyncWithOptions(&MyEvent{}, &MyListener{}, event.ListenOptions{
    Delay: time.Second * 5,  // 覆盖监听器的 10s
    Tries: 3,                // 覆盖监听器的 5
})
// 最终: Delay=5s, Tries=3

// ListenOptions 部分覆盖
event.ListenAsyncWithOptions(&MyEvent{}, &MyListener{}, event.ListenOptions{
    Delay: time.Second * 5,  // 覆盖
    // Tries 未设置，使用监听器的 5
})
// 最终: Delay=5s, Tries=5

// 不使用 ListenOptions
event.ListenAsync(&MyEvent{}, &MyListener{})
// 最终: Delay=10s, Tries=5 (全部使用监听器配置)
```

## 🔥 失败处理

```go
// 设置失败处理器
dispatcher.SetFailedHandler(event.FailedJobHandlerFunc(func(ctx context.Context, job *event.Job, err error) error {
    // 记录到数据库
    log.Printf("Job failed: %s, error: %v", job.ID, err)

    // 发送告警
    alertService.Send(fmt.Sprintf("Event %s failed after %d attempts", job.EventName, job.Attempts))

    return nil
}))
```

## 📊 性能特性

- **并发安全**: 所有组件都是线程安全的
- **零拷贝**: 事件载荷在可能的情况下避免拷贝
- **连接池**: Redis/DB 驱动自动管理连接池
- **批处理**: 支持批量事件分发
- **延迟队列**: 基于时间轮或Redis ZSET实现

## 🧪 测试

```bash
# 运行所有测试
go test -v ./...

# 查看覆盖率
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# 运行基准测试
go test -bench=. -benchmem
```

## 📚 完整示例

查看 [`_examples/`](./_examples/) 目录获取完整的使用示例。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 License

MIT License

## 🙏 致谢

本项目基于以下优秀的开源项目构建：

- [Watermill](https://github.com/ThreeDotsLabs/watermill) - 事件驱动架构库
- [Zap](https://github.com/uber-go/zap) - 高性能日志库
- [Wire](https://github.com/google/wire) - 依赖注入代码生成器
