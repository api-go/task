# @go/task

> **Maintainer Statement:** 本项目完全由 AI 维护。任何改动均遵循代码质量与性能的最佳实践。

## 🎯 设计哲学

`@go/task` 是一个简单、易用且高效的任务调度引擎。它建立在 `robfig/cron` 之上，提供了更丰富的任务管控能力，如并发策略控制、生命周期管理和超时控制。设计严格遵循单一职责原则（SRP），剔除了不相关的状态共享模块，让任务调度更加专注。

**v1.2.0 重大更新**：基础设施对齐，`Scheduler` 实现 `Service` 接口，移除顶层 `Start/Stop` 函数。

## 📦 安装

```bash
go get apigo.cc/go/task
```

## 💡 快速开始

### 1. 注册任务
任务函数必须遵循 `func(ctx context.Context) error` 签名，以便处理超时和优雅退出。

```go
import "apigo.cc/go/task"

// 简单注册（使用默认配置）
task.Add("CleanLog", "@daily", func(ctx context.Context) error {
    // 执行清理逻辑
    return nil
})

// 带配置注册
task.Add("Report", "@every 1m", func(ctx context.Context) error {
    // ...
    return nil
}, task.Config{
    Policy:             task.PolicySkip,    // 如果上次还没跑完，跳过本次
    Timeout:            5 * time.Second,    // 单次执行超时
    SuppressSuccessLog: true,               // 抑制默认 success 日志，由任务自行汇报
})
```

### 2. 生命周期管理
`Scheduler` 实现了标准的 `Service` 接口，推荐通过基础设施管理。

```go
// 启动默认调度器
task.DefaultScheduler.Start(ctx, logger)

// 手动触发任务执行
task.Run("CleanLog")

// 优雅停止：取消所有任务 Context，并等待任务返回（由 ctx 控制超时）
task.DefaultScheduler.Stop(ctx)
```

### 3. 任务控制
可以通过对象或全局方法管理任务状态。

```go
tk := task.Get("CleanLog")

tk.Disable()  // 挂起任务
tk.Enable()   // 恢复任务
tk.Remove()   // 彻底移除

task.Disable("CleanLog") // 按名称挂起，未知任务返回 false
task.Enable("CleanLog")  // 按名称恢复，未知任务返回 false

// 查询
tasks := task.List()
```

### 4. 任务上下文日志

调度器从生命周期管理器接收父 Logger，并为每次实际执行创建独立 Trace。任务函数可直接从 Context 取得本次运行的 Logger：

```go
task.Add("Report", "@every 1m", func(ctx context.Context) error {
    logger := task.Logger(ctx)
    logger.Info("report generated", "count", 12)
    return nil
})
```

空轮询任务可使用 `SuppressSuccessLog` 避免无效磁盘日志；失败、Panic 和未抑制的成功日志均沿用本次运行的 Trace。

### 5. Redis 可靠队列

`task/queue` 按全局队列名使用约定连接 `redis.task`，调用方无需持有 Redis Client 或 Context：

```go
import "apigo.cc/go/task/queue"

_ = queue.Add("translation", item)
items, _ := queue.Fetch("translation", "default", 50)
// processing 中存在未完成批次时，Fetch 忽略 n 并返回原批次。
// 业务成功后一次性确认当前消费者的 processing。
_ = queue.Finish("translation", "default")

// 持久化延迟任务；到期后由 Fetch 提升到 pending。
_ = queue.AddAfter("translation", item, 30*time.Second)

// Retry/Fail 只支持 Fetch(..., 1) 的单项 processing，拒绝批量误确认。
// 先保存重试项，再确认 processing；delay 以毫秒计。
_ = queue.Retry("translation", "default", item, 5000)
// 重试次数由宿主控制，最终失败项保留在 translation:failed。
_ = queue.Fail("translation", "default", item)
```

队列使用 `pending`、按消费者隔离的 `processing` 和 Sorted Set `delayed` 三条持久化通道。任务失败时不调用 `Finish`，下一次 `Fetch` 会恢复同一 processing 批次。

`Stats` 只读取 pending、processing、delayed 和 failed 数量；`PeekFailed(queueName, offset, limit)` 分页只读终态失败项，不会确认或重新投递任务，适合宿主按失败来源抑制重复恢复。`FetchDecoded` 会将坏 JSON 保存到 `队列名:failed`，混合批次中的有效任务重新入队后才确认原批次。每个 consumerID 应由一个独占消费者使用；崩溃恢复采用至少一次交付，宿主必须保证幂等。

`Fetch` 使用标准 `LMOVE` 原子地把任务转入 processing。为兼容要求目标 List 预先存在的 SugarDB，首次移动前会用一个立即弹出的内部标记初始化空 List；该标记不会保留在 processing，也不会返回给调用方。标准 Redis 会在弹出最后一个元素后删除 key，随后的 `LMOVE` 会按标准语义创建目标 List。

## 🛡️ 健壮性与安全

*   **Panic Recovery**: 框架内部自动捕获任务执行中的 Panic，并记录堆栈日志，确保调度引擎持续稳定。
*   **Context 传播**: 任务内部应监听 `ctx.Done()` 以响应系统的停止信号。
*   **标准化日志**: 集成 `@go/log`，为每次实际执行创建独立 Trace，并记录成功、失败（含耗时）以及 Panic 信息。

## 🧪 验证状态
测试全部通过，性能达标。

详见：[TEST.md](./TEST.md)


## indexing recovery

Config.FailureRetryDelay optionally replaces consecutive-failure auto-disable with exponential cooldown (up to five minutes). RetryAt is exposed in runtime snapshots. Successful execution clears cooldown; explicit Disable remains authoritative. The default zero value preserves existing scheduler behavior.
