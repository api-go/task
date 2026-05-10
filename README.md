# @go/task

> **Maintainer Statement:** 本项目完全由 AI 维护。任何改动均遵循代码质量与性能的最佳实践。

## 🎯 设计哲学

`@go/task` 是一个简单、易用且高效的任务调度引擎。它建立在 `robfig/cron` 之上，提供了更丰富的任务管控能力，如并发策略控制、生命周期管理和超时控制。设计严格遵循单一职责原则（SRP），剔除了不相关的状态共享模块，让任务调度更加专注。

## 📦 安装

```bash
go get apigo.cc/go/task
```

## 💡 核心功能

### 1. 快速注册与自动启动
支持标准的 Cron 表达式。模块加载时自动启动了极低资源占用的默认调度器，无需手动 `Start()`。

```go
// Add 返回任务操作句柄
tk := task.Add("CleanLog", "@daily", func() {
    // 执行清理逻辑
})
```

### 2. 生命周期与状态控制
使用面向对象的方法进行控制，保持命名空间整洁。

```go
tk := task.Get("CleanLog")

tk.Disable()  // 挂起任务（到了时间也不执行）
tk.Enable()   // 恢复任务
tk.Remove()   // 从调度引擎中彻底移除

// 查询任务
tasks := task.List() // 返回当前所有任务列表（包含运行状态）
```

### 3. 并发策略控制 (Policy)
控制当一个任务正在运行时，下一个调度周期触发时的行为。

- `PolicyParallel` (默认): 并行执行。
- `PolicySkip`: 如果上次任务仍在运行，则跳过本次执行。
- `PolicyQueue`: 如果上次任务仍在运行，则将本次执行放入队列，等待上次执行完成后立即开始。

```go
// 跳过重叠执行
task.Add("Report", "@every 1m", func() {
    // ...
}, task.WithPolicy(task.PolicySkip))
```

### 4. 生命周期钩子
用于监控和审计任务执行情况。

```go
task.Add("SyncData", "0 0 * * *", func() {
    // ...
}, task.OnSuccess(func(d time.Duration) {
    // d 为任务耗时
}), task.OnError(func(err error) {
    // 任务执行报错（如果任务函数返回 error）
}))
```

### 5. 超时控制
支持任务执行的超时管理，防止发生僵尸任务。

```go
task.Add("FetchAPI", "@every 10s", func() {
    // ...
}, task.WithTimeout(5*time.Second))
```

## 🧪 验证状态
测试全部通过，性能达标。

详见：[TEST.md](./TEST.md)
