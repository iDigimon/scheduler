package scheduler

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/iDigimon/scheduler/internal/heap"
)

const minRepeatInterval = time.Minute

// JobChecker 在派发前检查任务快照，返回 true 时结束任务。
type JobChecker func(job *Job) bool

// UntilTime 在计划执行时刻早于 t 时结束任务。
func UntilTime(t time.Time) JobChecker {
	return func(job *Job) bool { return job.NextRun.Before(t) }
}

// UntilDeadline 在计划执行时刻达到或超过截止时间时结束任务。
func UntilDeadline(t time.Time) JobChecker {
	return func(job *Job) bool { return !job.NextRun.Before(t) }
}

// UntilFreq 在已派发次数达到 execCount 时结束任务。
func UntilFreq(execCount int) JobChecker {
	return func(job *Job) bool { return job.ExecutionCount >= execCount }
}

type scheduledJob struct {
	Job
	nextCron func(time.Time) time.Time
}

// Scheduler 使用最小堆和一个定时器调度任务。
type Scheduler struct {
	mu     sync.RWMutex
	jobs   *heap.Heap[*scheduledJob]
	jobMap map[string]*scheduledJob
	timer  *time.Timer // 每次 Start 使用独立的定时器和 context
	cancel context.CancelFunc
}

// New 创建新的调度器。
func New() *Scheduler {
	s := &Scheduler{}
	s.resetJobs()
	return s
}

func (s *Scheduler) resetJobs() {
	s.jobs = heap.New(func(i, j *scheduledJob) bool { return i.NextRun.Before(j.NextRun) })
	s.jobMap = make(map[string]*scheduledJob)
}

// Start 启动调度器，派发本轮登记的任务。运行中重复调用只启动一次。
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.timer = time.NewTimer(0)
	s.updateTimer()
	go s.run(ctx, s.timer)
}

// Stop 停止运行中的调度器并清空本轮任务，已派发的回调可继续运行。
// 停止后可为下一次 Start 登记新任务。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.timer.Stop()
	s.cancel = nil
	s.timer = nil
	s.resetJobs()
}

// RunAt 在指定时间执行一次任务。
func (s *Scheduler) RunAt(id, name string, t time.Time, fn func()) error {
	return s.addJob(&scheduledJob{Job: Job{ID: id, Name: name, Func: fn, NextRun: t}})
}

// Every 按固定间隔循环派发任务，首次计划执行时刻为 startTime 加 interval。
// interval 小于 60 秒时，注册失败并返回 ErrInvalidInterval。
func (s *Scheduler) Every(id, name string, startTime time.Time, interval time.Duration, fn func(), checkers ...JobChecker) error {
	if interval < minRepeatInterval {
		return ErrInvalidInterval
	}
	job := Job{
		ID: id, Name: name, Func: fn,
		NextRun: startTime.Add(interval), Interval: interval, Checkers: checkers,
	}
	return s.addJob(&scheduledJob{Job: job})
}

// Crontab 使用“分 时 日 月 周”五段式表达式调度任务。
// 带秒字段的表达式在注册时返回 ErrInvalidCrontab。
func (s *Scheduler) Crontab(id, name, crontab string, fn func(), checkers ...JobChecker) error {
	next, err := ParseCrontab(crontab)
	if err != nil {
		return err
	}
	t, err := nextCronTime(next, time.Now())
	if err != nil {
		return err
	}
	return s.addJob(&scheduledJob{
		Job:      Job{ID: id, Name: name, Func: fn, NextRun: t, Crontab: crontab, Checkers: checkers},
		nextCron: next,
	})
}

func (s *Scheduler) addJob(job *scheduledJob) error {
	if job.Func == nil {
		return ErrNilJobFunc
	}
	for _, check := range job.Checkers {
		if check == nil {
			return ErrNilJobChecker
		}
	}
	job.Checkers = slices.Clone(job.Checkers)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobMap[job.ID]; exists {
		return ErrJobExists
	}
	s.jobs.Push(job)
	s.jobMap[job.ID] = job
	if s.jobs.Top() == job {
		s.updateTimer()
	}
	return nil
}

// Remove 移除指定任务。已派发的回调可继续运行。
func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.jobMap[id]
	if !exists {
		return ErrJobNotFound
	}
	if s.jobs.Top() == job {
		s.jobs.Pop()
		s.updateTimer()
	} else {
		// 正在执行检查器的任务已经出堆，此时只需撤销 map 中的登记。
		index := s.jobs.FindIndex(func(j *scheduledJob) bool { return j == job })
		s.jobs.Remove(index)
	}
	delete(s.jobMap, id)
	return nil
}

// FindJob 根据 ID 返回任务快照。
func (s *Scheduler) FindJob(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, exists := s.jobMap[id]
	if !exists {
		return nil, ErrJobNotFound
	}
	return job.snapshot(), nil
}

// Jobs 获取所有待执行任务的信息。
func (s *Scheduler) Jobs() []JobInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]JobInfo, 0, len(s.jobMap))
	for _, job := range s.jobMap {
		result = append(result, JobInfo{ID: job.ID, Name: job.Name, NextRun: job.NextRun, Interval: job.Interval})
	}
	return result
}

func (s *Scheduler) run(ctx context.Context, timer *time.Timer) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.executeReadyJobs(ctx)
		}
	}
}

func (s *Scheduler) executeReadyJobs(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	now := time.Now()
	for job := s.jobs.Top(); job != nil && !job.NextRun.After(now); job = s.jobs.Top() {
		s.jobs.Pop()
		if len(job.Checkers) == 0 {
			s.dispatch(job)
		} else {
			// 检查期间仅保留 map 登记，同一任务最多有一轮检查在运行。
			go s.checkJob(ctx, job, job.Job)
		}
	}
	s.updateTimer()
}

// checkJob 在锁外按顺序调用当前任务的检查器，每个检查器使用独立快照。
func (s *Scheduler) checkJob(ctx context.Context, job *scheduledJob, snapshot Job) {
	stop := true
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("任务 %s 的检查器执行出错: %v\n", snapshot.Name, r)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.jobMap[snapshot.ID] != job {
			return // 检查期间已停止调度、删除任务或复用 ID
		}
		if stop {
			delete(s.jobMap, snapshot.ID)
		} else {
			s.dispatch(job)
		}
		s.updateTimer()
	}()

	for _, check := range snapshot.Checkers {
		if ctx.Err() != nil || check(snapshot.snapshot()) {
			return
		}
	}
	stop = false
}

// dispatch 持写锁记录派发并安排下次执行，工作 goroutine 只接收名称和回调。
func (s *Scheduler) dispatch(job *scheduledJob) {
	now := time.Now()
	job.LastRun = now
	job.ExecutionCount++
	go runJob(job.Name, job.Func)

	switch {
	case job.Interval > 0:
		job.NextRun = now.Add(job.Interval)
	case job.nextCron != nil:
		next, err := nextCronTime(job.nextCron, now)
		if err != nil {
			delete(s.jobMap, job.ID)
			return
		}
		// 上次派发可能晚于整分；保留下个匹配分钟，同时保证实际派发间隔。
		earliest := now.Add(minRepeatInterval)
		if next.Before(earliest) {
			next = earliest
		}
		job.NextRun = next
	default:
		delete(s.jobMap, job.ID)
		return
	}
	s.jobs.Push(job)
}

func runJob(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("任务 %s 执行出错: %v\n", name, r)
		}
	}()
	fn()
}

// updateTimer 持写锁将当前代次的定时器对准堆顶，空堆时休眠。
func (s *Scheduler) updateTimer() {
	if s.timer == nil {
		return
	}
	if job := s.jobs.Top(); job != nil {
		s.timer.Reset(time.Until(job.NextRun))
	} else {
		s.timer.Stop()
	}
}

// JobInfo 表示任务的基本调度信息。
type JobInfo struct {
	ID       string
	Name     string
	NextRun  time.Time
	Interval time.Duration
}
