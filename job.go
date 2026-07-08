package scheduler

import (
	"fmt"
	"time"
)

// 错误定义
var (
	ErrJobExists       = fmt.Errorf("job with this ID already exists")
	ErrJobNotFound     = fmt.Errorf("job not found")
	ErrInvalidInterval = fmt.Errorf("interval must be at least one minute")
)

// Job 表示一个任务
type Job struct {
	ID             string
	Name           string
	Func           func()
	NextRun        time.Time
	Interval       time.Duration // 0表示只执行一次
	Crontab        string        // crontab表达式，用于cron任务
	ExecutionCount int           // 执行次数
	LastRun        time.Time     // 上次执行时间
	Checkers       []JobChecker  // 任务检查器列表，用于判断任务是否应该继续执行
}