package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iDigimon/scheduler/internal/heap"
)

type JobChecker func(job *Job) bool

func UntilTime(t time.Time) JobChecker {
	return func(job *Job) bool {
		return job.NextRun.Before(t)
	}
}
func UntilFreq(execCount int) JobChecker {
	return func(job *Job) bool {
		return job.ExecutionCount >= execCount
	}
}

// Scheduler 调度器
type Scheduler struct {
	jobs    *heap.Heap[*Job]
	mu      sync.RWMutex
	timer   *time.Timer
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	jobMap  map[string]*Job // 用于快速查找任务
	wakeup  chan struct{}   // 用于唤醒调度器主循环的通道
}

// New 创建新的调度器
func New() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建以NextRun时间为比较条件的最小堆
	jobHeap := heap.New(func(i, j *Job) bool {
		return i.NextRun.Before(j.NextRun)
	})

	scheduler := &Scheduler{
		jobs:   jobHeap,
		ctx:    ctx,
		cancel: cancel,
		jobMap: make(map[string]*Job),
		wakeup: make(chan struct{}, 1), // 带缓冲的通道，避免阻塞
	}

	return scheduler
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	// 初始化定时器，确保不为空
	s.updateTimer()
	s.mu.Unlock()
	go s.run()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	s.cancel()
	if s.timer != nil {
		s.timer.Stop()
	}
}

// RunAt 在指定时间执行任务
func (s *Scheduler) RunAt(id, name string, t time.Time, fn func()) error {
	job := &Job{
		ID:      id,
		Name:    name,
		Func:    fn,
		NextRun: t,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在相同ID的任务
	if _, exists := s.jobMap[id]; exists {
		return ErrJobExists
	}

	// 使用优化的任务添加方法
	s.addJob(job)

	return nil
}

// Every 按固定间隔循环执行任务，支持可变参数的JobChecker
func (s *Scheduler) Every(id, name string, startTime time.Time, interval time.Duration, fn func(), checkers ...JobChecker) error {
	// 最低一分钟
	if interval < time.Minute {
		return ErrInvalidInterval
	}

	job := &Job{
		ID:       id,
		Name:     name,
		Func:     fn,
		NextRun:  startTime.Add(interval),
		Interval: interval,
		Checkers: checkers,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在相同ID的任务
	if _, exists := s.jobMap[id]; exists {
		return ErrJobExists
	}

	// 使用优化的任务添加方法
	s.addJob(job)

	return nil
}

// Remove 移除指定任务
func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.jobMap[id]
	if !exists {
		return ErrJobNotFound
	}

	// 在堆中找到该任务并移除
	index := s.jobs.FindIndex(func(j *Job) bool {
		return j.ID == id
	})

	if index >= 0 {
		s.jobs.Remove(index)
	}

	delete(s.jobMap, id)
	s.updateTimer()

	return nil
}

// FindJob 根据ID查找任务并返回任务指针
// 如果任务不存在，返回nil和ErrJobNotFound错误
func (s *Scheduler) FindJob(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.jobMap[id]
	if !exists {
		return nil, ErrJobNotFound
	}

	// 返回任务的副本，避免外部直接修改内部状态
	// 使用值复制而不是指针复制，确保返回的是新的指针
	jobCopy := *job
	return &jobCopy, nil
}

// Crontab 使用crontab表达式创建定时任务，支持可变参数的JobChecker
// id: 任务ID
// name: 任务名称
// crontab: crontab表达式，格式为"分 时 日 月 周"
// fn: 要执行的任务函数
// checkers: 任务检查器列表，用于判断任务是否应该继续执行
// 返回错误信息
func (s *Scheduler) Crontab(id, name, crontab string, fn func(), checkers ...JobChecker) error {
	// 解析crontab表达式
	nextRunFunc, err := ParseCrontab(crontab)
	if err != nil {
		return fmt.Errorf("invalid crontab expression: %w", err)
	}

	// 获取下一次执行时间
	nextRun := nextRunFunc(time.Now())
	if nextRun.IsZero() {
		return errors.New("failed to calculate next run time")
	}

	// 创建任务
	job := &Job{
		ID:       id,
		Name:     name,
		Func:     fn,
		NextRun:  nextRun,
		Crontab:  crontab,
		Checkers: checkers,
	}

	// 添加任务到调度器
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在相同ID的任务
	if _, exists := s.jobMap[id]; exists {
		return ErrJobExists
	}

	// 使用优化的任务添加方法
	s.addJob(job)

	return nil
}

// Jobs 获取所有任务信息
func (s *Scheduler) Jobs() []JobInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]JobInfo, 0, len(s.jobMap))
	for _, job := range s.jobMap {
		result = append(result, JobInfo{
			ID:       job.ID,
			Name:     job.Name,
			NextRun:  job.NextRun,
			Interval: job.Interval,
		})
	}

	return result
}

// run 调度器主循环
func (s *Scheduler) run() {
	for {
		s.mu.Lock()
		currentTimer := s.timer
		s.mu.Unlock()

		// 如果定时器为nil，重新初始化
		if currentTimer == nil {
			s.mu.Lock()
			s.updateTimer()
			s.mu.Unlock()
			continue
		}

		// 等待定时器触发、上下文取消或唤醒信号
		select {
		case <-s.ctx.Done():
			return
		case <-currentTimer.C:
			s.executeReadyJobs()
			// 执行完任务后，必须重新获取timer以确保使用最新的定时器
			continue
		case <-s.wakeup:
			// 收到唤醒信号，继续循环以获取新的定时器
		}
	}
}

// executeReadyJobs 执行到期的任务
func (s *Scheduler) executeReadyJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for s.jobs.Len() > 0 {
		job := s.jobs.Top()

		// 如果最早的任务还没到执行时间，则退出
		if job.NextRun.After(now) {
			break
		}

		// 移除任务
		s.jobs.Pop()

		// 检查是否应该继续执行该任务
		shouldContinue := true
		for _, checker := range job.Checkers {
			if checker(job) {
				shouldContinue = false
				break
			}
		}

		// 异步执行任务
		if shouldContinue {
			go func(j *Job) {
				defer func() {
					if r := recover(); r != nil {
						// panic 仅打印日志，不传播，避免中断调度循环
						fmt.Printf("任务 %s 执行出错: %v\n", j.Name, r)
					}
				}()
				// 记录任务执行时间
				j.LastRun = time.Now()
				j.Func()
				// 增加执行次数
				j.ExecutionCount++
			}(job)

			// 根据任务类型决定是否重新调度
			if job.Interval > 0 {
				// 间隔循环任务
				job.NextRun = now.Add(job.Interval)
				s.jobs.Push(job)
			} else if job.Crontab != "" {
				// Crontab 任务
				nextRunFunc, err := ParseCrontab(job.Crontab)
				if err == nil {
					nextRun := nextRunFunc(now)
					if !nextRun.IsZero() {
						job.NextRun = nextRun
						s.jobs.Push(job)
					}
				}
			} else {
				// 一次性任务，从映射中删除
				delete(s.jobMap, job.ID)
			}
		} else {
			// 从jobMap中删除已结束的任务
			delete(s.jobMap, job.ID)
		}
	}
	s.updateTimer()
}

// updateTimer 更新定时器
// updateTimer 无条件更新定时器
// 注意：此方法会无条件停止旧定时器并创建新定时器
func (s *Scheduler) updateTimer() {
	// 停止旧定时器（如果存在）
	if s.timer != nil {
		s.timer.Stop()
	}

	if s.jobs.Len() == 0 {
		s.timer = time.NewTimer(time.Hour * 24 * 365) // 默认等待1年
		return
	}

	nextJob := s.jobs.Top()
	duration := time.Until(nextJob.NextRun)
	if duration < 0 {
		duration = 0
	}

	s.timer = time.NewTimer(duration)

	// 唤醒主循环，让它重新获取新的定时器
	// 使用select避免阻塞（如果通道已满）
	select {
	case s.wakeup <- struct{}{}:
	default:
	}
}

// addJob 添加任务并优化定时器更新
// 只有当新添加的任务比当前最早任务更早时，才完全更新定时器
func (s *Scheduler) addJob(job *Job) {
	// 检查是否已存在相同ID的任务
	if _, exists := s.jobMap[job.ID]; exists {
		return
	}

	// 获取当前最早任务（如果有）
	hasExistingJobs := s.jobs.Len() > 0
	var earliestJob *Job
	if hasExistingJobs {
		earliestJob = s.jobs.Top()
	}

	// 添加新任务到堆和映射
	s.jobs.Push(job)
	s.jobMap[job.ID] = job

	// 优化逻辑：只有在以下情况才完全更新定时器
	// 1. 没有现有任务（首次添加任务）
	// 2. 新添加的任务比当前最早任务更早
	if !hasExistingJobs || job.NextRun.Before(earliestJob.NextRun) {
		s.updateTimer()
	} else {
		// 即使不需要完全更新定时器，也发送唤醒信号让主循环确认当前状态
		select {
		case s.wakeup <- struct{}{}:
		default:
		}
	}

}

// JobInfo 任务信息
type JobInfo struct {
	ID       string
	Name     string
	NextRun  time.Time
	Interval time.Duration
}
