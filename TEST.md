# Test Report

## v1.5.13 发布验证（2026-10-06）

- `GOWORK=off go test -v ./...` 与 `GOWORK=off go test -bench=. ./...` 均通过，仅使用 go.mod 已发布依赖。
- API 导出工具不可用；采用 `GOWORK=off go doc -all` 与导出源码交叉核对 README。
- 本机：darwin/arm64，Apple M3 Max；基准结果仅代表本次本地运行。
- `BenchmarkTaskRegistration-16         	 7246221	       153.9 ns/op`
- `BenchmarkTaskExecutionOverhead/Get-16         	158872885	         7.561 ns/op`
- `BenchmarkTaskExecutionOverhead/List-16        	17933064	        66.71 ns/op`

## v1.5.12 验证
- 验证首次 Stop 超时后，后续 Stop 继续等待在途任务完成。
- `TestTaskTimeout` 边界用例连续 5 次通过；全量测试重跑与 benchmark 通过。

## v1.5.10 验证
- `go test -v ./...`：通过；在临时标准 Redis 8.8 实例上覆盖任务调度、运行控制和可靠队列行为，测试后已关闭实例。
- `go test -bench=. ./...`：通过；`BenchmarkTaskRegistration-16` 为 `138.8 ns/op`，Task Get/List 分别为 `7.614 ns/op`、`67.71 ns/op`（Apple M3 Max）。
- 队列测试验证 processing 恢复、批量 Fetch/Finish、compatibility marker 不残留，以及 Sorted Set 延迟任务只在到期后提升；同一组测试已在未修改的 SugarDB 实例上通过。
- 现有调度测试覆盖对象级 Enable/Disable；包级 `Enable`/`Disable` 和运行级 Logger 传导已通过编译与 master 集成，尚缺独立单元测试。
- `git diff --check`：通过。

## v1.5.6 验证
- 独立模块解析、全量测试与基准测试均通过。

## Environment
- **Date**: 2026-06-22
- **Go Version**: go 1.25.0
- **OS**: darwin/amd64

## Summary
| Test Case | Status | Duration |
|-----------|--------|----------|
| TestTaskBasic | PASS | 2.50s |
| TestTaskLifecycle | PASS | 4.00s |
| TestTaskPolicySkip | PASS | 3.50s |
| TestTaskTimeout | PASS | 2.50s |
| TestTaskPanicRecover | PASS | 2.50s |
| TestTaskManualRun | PASS | 0.20s |
| TestGracefulStop | PASS | 1.00s |

## Benchmarks
| Benchmark | Iterations | ns/op |
|-----------|------------|-------|
| BenchmarkTaskRegistration | 3664093 | 318.1 |
| BenchmarkTaskExecutionOverhead/Get | 52381898 | 26.07 |
| BenchmarkTaskExecutionOverhead/List | 10123669 | 116.2 |

## Details

### TestGracefulStop
Successfully verified that the scheduler waits for running tasks to complete during `Stop(ctx)` using manual triggers for higher reliability.

### TestTaskPanicRecover
Verified that the scheduler remains operational and continues to schedule tasks even if a specific task panics, with full stack trace logging.

### TestTaskTimeout
Verified that the task-specific `context.Context` is correctly cancelled when the configured timeout is exceeded.

## Conclusion
All 7 test cases passed. The implementation of the `Service` interface is robust and optimized for performance.


## v1.5.13 indexing recovery

TestFailureCooldownPreservesExplicitPauseAndRecovers covers cooldown, explicit pause, success reset and automatic retry eligibility.

## v1.5.14 发布验证（2026-10-10）

- GOWORK=off go test -v ./... 与 go test -bench=. ./... 通过，依赖均为已发布 tag。
- API 以 GOWORK=off go doc 与导出源码交叉核对（getExports 工具不可用）。
- GOWORK=off go test -race ./... 通过。

```text
BenchmarkTaskRegistration-16         	 7303446	       166.0 ns/op
BenchmarkTaskExecutionOverhead/Get-16         	153838617	         7.843 ns/op
BenchmarkTaskExecutionOverhead/List-16        	16542891	        70.36 ns/op
```

## v1.5.15 发布验证（2026-10-26）

GOWORK=off，正式依赖 crypto v1.5.9，无 replace。`go test -v ./...`、`go test -bench=. ./...`、`go vet ./...`、`go mod verify` 通过。使用 `go doc` 与源码交叉核对 API，替代 getExports。现有回归覆盖本模块业务；新旧 GCM 格式、认证失败和性能基准见 crypto v1.5.9。

本次功能测试无跳过。

基准结果：
```text
BenchmarkTaskRegistration-16         	 7430384	       162.6 ns/op
BenchmarkTaskExecutionOverhead/Get-16         	153564003	         7.851 ns/op
BenchmarkTaskExecutionOverhead/List-16        	17104585	        70.02 ns/op
```
