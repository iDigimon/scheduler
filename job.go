package scheduler

import (
	"errors"
	"slices"
	"time"
)

var (
	ErrJobExists       = errors.New("job with this ID already exists")
	ErrJobNotFound     = errors.New("job not found")
	ErrInvalidInterval = errors.New("interval must be at least one minute")
	ErrNilJobFunc      = errors.New("job function cannot be nil")
	ErrNilJobChecker   = errors.New("job checker cannot be nil")
	ErrInvalidCrontab  = errors.New("invalid crontab expression")
	ErrNoNextRun       = errors.New("failed to calculate next run time")
)

// Job 表示一个任务。
type Job struct {
	ID             string
	Name           string
	Func           func()
	NextRun        time.Time
	Interval       time.Duration // 固定间隔任务的执行周期
	Crontab        string        // Cron 任务的表达式
	ExecutionCount int           // 已派发次数，包含正在执行或发生 panic 的任务
	LastRun        time.Time     // 上次派发时间
	Checkers       []JobChecker  // 每次派发前检查，任一返回 true 即终止任务
}

func (j Job) snapshot() *Job {
	j.Checkers = slices.Clone(j.Checkers)
	return &j
}
