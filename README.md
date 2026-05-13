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
    Policy:  task.PolicySkip,    // 如果上次还没跑完，跳过本次
    Timeout: 5 * time.Second,   // 单次执行超时
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

// 查询
tasks := task.List()
```

## 🛡️ 健壮性与安全

*   **Panic Recovery**: 框架内部自动捕获任务执行中的 Panic，并记录堆栈日志，确保调度引擎持续稳定。
*   **Context 传播**: 任务内部应监听 `ctx.Done()` 以响应系统的停止信号。
*   **标准化日志**: 集成 `@go/log`，自动记录每个任务的开始、成功、失败（含耗时）以及 Panic 信息。

## 🧪 验证状态
测试全部通过，性能达标。

详见：[TEST.md](./TEST.md)
