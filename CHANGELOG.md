# Changelog

## v1.5.10 - 2026-09-05
- **依赖对齐**: 补齐 `config`、`file`、`log` 与 `redis` 的级联版本。

## v1.5.9 - 2026-09-05
- **依赖对齐**: 升级全部 `apigo.cc/go/*` 依赖至当前已发布版本。

## v1.5.8 - 2026-08-31
- **失败保护**: 支持连续失败次数统计、自动停用回调及状态快照，避免任务持续消耗资源。

## v1.5.7 - 2026-08-27
- **可靠队列**: 新增 `task/queue`，提供 `Add`、`Fetch`、`Finish`、`FetchCount` 与泛型 `Decode`；通过全局队列名和 `redis.task` 配置契约管理 pending 及消费者隔离的 processing。
- **Redis 兼容**: 保持标准 `LMOVE` 原子移动语义，并通过不保留数据的空 List 初始化兼容 SugarDB 对目标 List 预先存在的要求。
- **延迟任务**: 新增 `AddAfter` 与 `PromoteDelayed`，使用 Redis Sorted Set 按到期时间提升任务，避免消费者反复读取尚未到期的重试项。
- **运行控制**: 新增包级 `Enable(name)`、`Disable(name)`，便于管理端先操作运行状态、成功后再持久化恢复状态。
- **日志传导**: 调度器继承生命周期 Logger，为每次任务运行创建独立 Trace，并通过 `WithLogger`、`Logger` 在 Context 中传导；空轮询不额外产生启动日志。
- **依赖更新**: 升级 `redis` 至 `v1.5.13`、`jsmod` 至 `v1.5.4`。

## v1.5.6 - 2026-07-18
- **依赖校验修复**: 更新 `go.sum` 为当前可验证的依赖 checksum。

## v1.5.5 - 2026-06-22
- **新增**: `Config.SuppressSuccessLog` 字段，设为 true 时调度器跳过 success 日志，由任务函数自行按处理量汇报，避免空轮询日志噪音。

## v1.5.4 - 2026-06-21
- **依赖更新**: 升级依赖 `jsmod` 至 `v1.5.3`，`cast` 至 `v1.5.3`，`rand` 至 `v1.5.3`，`encoding` 至 `v1.5.4`，`shell` 至 `v1.5.3`，`safe` 至 `v1.5.2`，`id` 至 `v1.5.4`，`file` 至 `v1.5.5`，`config` 至 `v1.5.3`，`log` 至 `v1.5.8`。

## v1.5.2 - 2026-06-05
- **优化: 生命周期日志清理**:
  - 移除了内部冗余的 `scheduler started` 与 `scheduler stopped gracefully` 控制台输出。这些状态现由 `apigo.cc/go/starter` 框架统一劫持并提供标准化的生命周期审计。

## v1.5.1 - 2026-06-04
- **依赖升级**: 全面对接 `@go` 基础设施 v1.5.1。

## v1.5.0 - 2026-05-10
- **基础设施对齐**: 全局对齐至 v1.5.0。

## v1.3.0 - 2026-05-14

- **基础设施对齐 (Infrastructure Alignment)**:
    - 升级 `apigo.cc/go/log` 至 `v1.3.2`。
    - 引入 `apigo.cc/go/timer` 模块支持 (预留用于未来更精细的时间调度)。
- **代码重构与类型对齐**:
    - 将 `Task.entryID` 类型从 `int` 修改为更严谨的 `cron.EntryID`。
    - 移除不必要的类型转换。
- **性能优化与基准测试**:
    - 新增 Benchmark 覆盖任务注册、获取与列表操作。
    - 优化了测试用例的稳定性，特别是优雅退出的验证。

## v1.2.0 - 2026-05-12

- **基础设施对齐 (Service Interface)**: `Scheduler` 现在实现了标准 `Service` 接口 (`Start`, `Stop`, `Health`)。
- **API 变更 (Breaking Change)**: 移除顶层 `task.Start()` 和 `task.Stop()` 函数。必须通过 `task.DefaultScheduler` 进行生命周期管理。
- **生命周期优化**:
    - `Start(ctx, logger)` 支持传入自定义 Logger。
    - `Stop(ctx)` 现在由调用方通过 `context` 控制优雅退出的超时时间。
- **健壮性增强**: 增加 `Health()` 接口用于基础设施监控。

## v1.1.0 - 2026-05-10

- **API 重构 (Breaking Change)**: `Add` 签名变更为 `Add(name, spec, func(context.Context) error, ...Config)`。
- **配置模式优化**: 引入 `Config` 结构体替代 Functional Options，遵循 AI-First 极简主义。
- **生命周期增强**: 
    - 增加显式的 `Start()` 和 `Stop()` 方法。
    - `Stop()` 支持优雅退出，自动取消任务 Context 并等待运行中的任务完成（默认 30s 超时）。
- **健壮性提升**: 
    - 内置 `recover` 机制，防止单个任务 Panic 导致调度器或进程崩溃。
    - 移除冗余且危险的 `PolicyQueue` 策略，推荐使用 `PolicySkip` 结合外部队列。
- **基础设施对齐**: 
    - 集成 `@go/log` 实现标准化的异步元数据日志记录。
    - 对齐 `apigo.cc/go/cast` 和 `apigo.cc/go/log` v1.3.0 标准。

## v1.0.0

- `New` 核心调度器实现，支持并发策略 (Parallel, Skip, Queue)。
- `New` 支持生命周期钩子 (OnSuccess, OnError) 和超时控制。
- `New` 支持通过对象方法对任务进行管理 (Disable, Enable, Remove)。
- `New` 支持全局查询任务列表和任务状态。
