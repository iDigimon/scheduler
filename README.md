# scheduler

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

一个轻量、零依赖的 Go 定时任务调度库。基于**最小堆**管理任务的下次执行时间，使用单个自适应定时器驱动调度循环，支持一次性任务、固定间隔循环任务和 Crontab 表达式任务，并可通过 `JobChecker` 灵活控制任务的终止条件。

## 特性

- ⏱️ **三种任务类型**：定点执行（`RunAt`）、循环间隔（`Every`）、Crontab 表达式（`Crontab`）
- 🧠 **最小堆调度**：以 `NextRun` 为关键字的最小堆，保证下次任务总在堆顶，O(log n) 入堆 / 出堆
- ⏳ **单定时器驱动**：仅维护一个 `time.Timer`，按堆顶任务的执行时间动态调整，空闲时几乎不占 CPU
- 🛡️ **并发安全**：所有对外方法通过 `sync.RWMutex` 保护；任务在独立 goroutine 中异步执行，panic 会被 recover 捕获
- 🔧 **灵活的终止条件**：通过可变参数 `JobChecker` 控制任务何时停止（执行次数、截止时间或任意自定义逻辑）
- 🗂️ **任务管理**：支持按 ID 查找、移除，以及获取全部任务快照
- 📦 **零依赖**：仅使用 Go 标准库

## 安装

```bash
go get github.com/iDigimon/scheduler
```

引入包：

```go
import "github.com/iDigimon/scheduler"
```

所有公开 API 位于 `scheduler` 包下，调用时使用前缀 `scheduler.`。

## 快速开始

```go
package main

import (
	"fmt"
	"time"

	"github.com/iDigimon/scheduler"
)

func main() {
	// 创建并启动调度器
	s := scheduler.New()
	s.Start()
	defer s.Stop()

	// 1) 一次性任务：5 秒后执行
	_ = s.RunAt("once", "一次性任务", time.Now().Add(5*time.Second), func() {
		fmt.Println("一次性任务执行:", time.Now().Format("15:04:05"))
	})

	// 2) 循环任务：每 1 分钟执行一次（间隔必须 >= 1 分钟）
	_ = s.Every("loop", "循环任务", time.Now(), time.Minute, func() {
		fmt.Println("循环任务执行:", time.Now().Format("15:04:05"))
	})

	// 3) Crontab 任务：每小时的第 0 分钟执行
	_ = s.Crontab("cron", "Cron任务", "0 * * * *", func() {
		fmt.Println("Cron任务执行:", time.Now().Format("15:04:05"))
	})

	// 观察执行
	time.Sleep(30 * time.Second)
}
```

## API 参考

### 调度器

| 方法 | 说明 |
| --- | --- |
| `New() *Scheduler` | 创建一个新的调度器实例 |
| `(s *Scheduler) Start()` | 启动调度器（后台 goroutine） |
| `(s *Scheduler) Stop()` | 停止调度器并取消上下文 |
| `(s *Scheduler) RunAt(id, name string, t time.Time, fn func()) error` | 在指定时间执行一次任务 |
| `(s *Scheduler) Every(id, name string, startTime time.Time, interval time.Duration, fn func(), checkers ...JobChecker) error` | 按固定间隔循环执行；`interval` 必须 >= 1 分钟 |
| `(s *Scheduler) Crontab(id, name, crontab string, fn func(), checkers ...JobChecker) error` | 使用 5 段式 crontab 表达式调度 |
| `(s *Scheduler) Remove(id string) error` | 按 ID 移除任务 |
| `(s *Scheduler) FindJob(id string) (*Job, error)` | 按 ID 查找任务（返回副本） |
| `(s *Scheduler) Jobs() []JobInfo` | 返回所有任务的快照信息 |

> 任务 ID 不可重复，相同 ID 会返回 `ErrJobExists`。

### JobChecker（终止条件）

`Every` 和 `Crontab` 支持传入任意数量的 `JobChecker`，**任一返回 `true` 即停止该任务**：

```go
type JobChecker func(job *Job) bool
```

内置：

- `scheduler.UntilTime(t time.Time) JobChecker` —— 当任务下次执行时间早于 `t` 时停止
- `scheduler.UntilFreq(execCount int) JobChecker` —— 当执行次数达到 `execCount` 时停止

自定义示例（分钟数为偶数时停止）：

```go
custom := func(job *scheduler.Job) bool {
	return time.Now().Minute()%2 == 0
}
s.Every("id", "name", time.Now(), time.Minute, fn, custom)
```

### Crontab 表达式

采用标准 5 段格式：`分 时 日 月 周`，例如：

| 表达式 | 含义 |
| --- | --- |
| `0 * * * *` | 每小时的第 0 分钟 |
| `0 0 * * *` | 每天午夜 |
| `*/5 * * * *` | （暂不支持，见下） |

工具函数：

- `scheduler.ParseCrontab(expr string) (func(time.Time) time.Time, error)` —— 解析表达式，返回下次执行时间计算函数
- `scheduler.GetNextRunTime(expr string) (time.Time, error)` —— 获取从当前时间起的下一次执行时间

> ⚠️ **当前限制**：crontab 解析目前仅支持 `*`（通配）和精确整数值，**暂不支持** 范围（`1-5`）、列表（`1,3,5`）、步长（`*/5`）等高级语法（见 `utils.go` 中的 TODO）。星期字段中周日既可用 `0` 也可用 `7` 表示。

## 任务执行模型

- 任务到点后从堆顶弹出，在**独立 goroutine** 中异步执行（不阻塞调度循环）。
- 执行过程中发生的 `panic` 会被 recover 捕获并打印，不会导致整个调度器崩溃。
- 执行完成后，循环任务 / Crontab 任务会重新计算 `NextRun` 并重新入堆；一次性任务或被 Checker 终止的任务会从 `jobMap` 中删除。
- 字段 `LastRun`、`ExecutionCount` 会在每次执行后更新。

## 项目结构

```
.
├── scheduler.go              # Scheduler 核心：任务调度、执行循环、定时器管理
├── job.go                    # Job 结构体与错误定义
├── utils.go                  # Crontab 表达式解析
├── example.go                # 使用示例（含 main 函数，非可执行程序）
├── scheduler_test.go         # 测试与示例
├── internal/
│   └── heap/
│       ├── min_heap.go       # 泛型最小堆 Heap[T] 实现
│       ├── heap_test.go      # 堆核心行为测试
│       ├── int_heap_test.go  # Heap[int] 特化场景与基准
│       └── ordered_heap_test.go # 有序类型用法与基准测试
├── go.mod
└── LICENSE
```

> 泛型最小堆 `Heap[T]` 位于 `internal/heap`，是独立通用组件，可独立测试（`go test ./internal/heap/`）。

## 开发

```bash
# 编译与静态检查（提交前请确保通过）
go build ./...
go vet ./...

# 运行测试（部分测试含较长 sleep，注意耗时）
go test ./...

# 运行单个调度器测试
go test -run TestJobChecker -v

# 只跑堆的测试 / 基准
go test ./internal/heap/ -v
go test -bench=. ./internal/heap/
```

## 许可证

[MIT](LICENSE) © iDigimon
