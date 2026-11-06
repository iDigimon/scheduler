package scheduler

import (
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestJobChecker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		block := make(chan struct{})
		defer close(block)
		var calls atomic.Int32
		start := time.Now()
		if err := s.Every("slow", "", start, time.Minute, func() {
			calls.Add(1)
			<-block
		}, UntilFreq(1)); err != nil {
			t.Fatal(err)
		}
		s.Start()
		defer s.Stop()
		time.Sleep(time.Minute)
		synctest.Wait()
		job, err := s.FindJob("slow")
		if err != nil {
			t.Fatal(err)
		}
		if job.ExecutionCount != 1 || !job.LastRun.Equal(start.Add(time.Minute)) || !job.NextRun.Equal(start.Add(2*time.Minute)) {
			t.Errorf("派发后的任务状态错误：%+v", job)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Errorf("任务执行了 %d 次", calls.Load())
		}
		if _, err := s.FindJob("slow"); !errors.Is(err, ErrJobNotFound) {
			t.Errorf("达到次数上限后查询得到 %v", err)
		}
	})
}

func TestCustomJobChecker(t *testing.T) {
	s := New()
	replaced := make(chan error, 1)
	fired := make(chan struct{}, 1)
	var stale atomic.Bool
	err := s.Every("id", "", time.Now().Add(-time.Minute), time.Minute, func() { stale.Store(true) }, func(j *Job) bool {
		if _, err := s.FindJob(j.ID); err != nil {
			replaced <- err
			return true
		}
		_ = s.Jobs()
		if err := s.Remove(j.ID); err != nil {
			replaced <- err
			return true
		}
		replaced <- s.RunAt(j.ID, "新任务", time.Now(), func() { fired <- struct{}{} })
		return false
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	timeout := time.After(3 * time.Second)
	select {
	case err := <-replaced:
		if err != nil {
			t.Fatal(err)
		}
	case <-timeout:
		t.Fatal("Checker 重入调度器超时")
	}
	select {
	case <-fired:
	case <-timeout:
		t.Fatal("同 ID 的新任务未执行")
	}
	s.Stop()
	if stale.Load() {
		t.Fatal("已移除的任务仍被派发")
	}
}

func TestCronDispatchOncePerMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		var calls atomic.Int32
		checks := 0
		if err := s.Crontab("cron", "", "* * * * *", func() { calls.Add(1) }, func(*Job) bool {
			checks++
			return checks > 3 // 限制错误实现可以连续派发的次数
		}); err != nil {
			t.Fatal(err)
		}
		s.Start()
		defer s.Stop()
		synctest.Wait()
		if calls.Load() != 0 {
			t.Fatal("在下一个整分钟之前派发了任务")
		}
		for i := int32(1); i <= 3; i++ {
			time.Sleep(time.Minute)
			synctest.Wait()
			if calls.Load() != i {
				t.Fatalf("第 %d 分钟共执行 %d 次", i, calls.Load())
			}
		}
	})
}

func TestSchedulerRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		now := time.Now()
		old := func() { t.Error("停止前的任务仍被派发") }
		if err := s.RunAt("once", "", now.Add(time.Minute), old); err != nil {
			t.Fatal(err)
		}
		if err := s.Every("every", "", now, time.Minute, old); err != nil {
			t.Fatal(err)
		}
		if err := s.Crontab("cron", "", "* * * * *", old); err != nil {
			t.Fatal(err)
		}
		s.Start()
		s.Stop()
		if jobs := s.Jobs(); len(jobs) != 0 {
			t.Fatalf("停止后仍有任务：%v", jobs)
		}
		s.Start()
		defer s.Stop()
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if jobs := s.Jobs(); len(jobs) != 0 {
			t.Fatalf("重启后旧任务重新出现：%v", jobs)
		}
	})
}

func TestCheckerSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		checkers := []JobChecker{func(j *Job) bool { j.ID = "改名"; return true }}
		if err := s.Every("id", "", time.Now(), time.Minute, func() { t.Error("任务应被终止") }, checkers...); err != nil {
			t.Fatal(err)
		}
		checkers[0] = nil
		j, err := s.FindJob("id")
		if err != nil {
			t.Fatal(err)
		}
		if j.Checkers[0] == nil {
			t.Fatal("注册后仍共享传入的切片")
		}
		j.Checkers[0] = nil
		j, err = s.FindJob("id")
		if err != nil || j.Checkers[0] == nil {
			t.Fatal("快照泄漏了内部切片")
		}
		s.Start()
		defer s.Stop()
		time.Sleep(time.Minute)
		synctest.Wait()
		if len(s.Jobs()) != 0 {
			t.Fatal("Checker 修改快照破坏了任务索引")
		}
	})
}

func TestDeadlineBoundaries(t *testing.T) {
	cutoff := time.Now()
	for _, tc := range []struct {
		offset           time.Duration
		before, deadline bool
	}{
		{-time.Nanosecond, true, false}, {0, false, true}, {time.Nanosecond, false, true},
	} {
		j := &Job{NextRun: cutoff.Add(tc.offset)}
		if UntilTime(cutoff)(j) != tc.before || UntilDeadline(cutoff)(j) != tc.deadline {
			t.Errorf("截止边界判定错误，偏移 %v", tc.offset)
		}
	}
}

func TestInvalidCallbacks(t *testing.T) {
	s := New()
	now := time.Now()
	for _, tc := range []struct{ err, want error }{
		{s.RunAt("a", "", now, nil), ErrNilJobFunc},
		{s.Every("b", "", now, time.Minute, nil), ErrNilJobFunc},
		{s.Crontab("c", "", "* * * * *", nil), ErrNilJobFunc},
		{s.Every("d", "", now, time.Minute, func() {}, nil), ErrNilJobChecker},
		{s.Crontab("e", "", "* * * * *", func() {}, nil), ErrNilJobChecker},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("得到 %v，期望 %v", tc.err, tc.want)
		}
	}
	if len(s.Jobs()) != 0 {
		t.Fatal("参数错误却添加了任务")
	}
}

func TestRejectFrequentJobsAtRegistration(t *testing.T) {
	s := New()
	now := time.Now()
	fn := func() {}
	if err := s.Every("id", "", now, time.Minute-time.Nanosecond, fn); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("不足 60 秒的间隔应在注册时被拒绝，得到 %v", err)
	}
	if err := s.Crontab("id", "", "*/30 * * * * *", fn); !errors.Is(err, ErrInvalidCrontab) {
		t.Fatalf("带秒字段的 Cron 应在注册时被拒绝，得到 %v", err)
	}
	if len(s.Jobs()) != 0 || s.jobs.Len() != 0 {
		t.Fatal("注册失败后仍有任务留在调度器内")
	}
	if err := s.Every("id", "", now, time.Minute, fn); err != nil {
		t.Fatalf("使用同一 ID 注册 60 秒间隔应成功，得到 %v", err)
	}
}

func TestCronFirstRunWithinOneMinuteOfRegistration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		s.Start()
		defer s.Stop()

		time.Sleep(30 * time.Second)
		registered := time.Now()
		var calls atomic.Int32
		if err := s.Crontab("id", "", "* * * * *", func() { calls.Add(1) }); err != nil {
			t.Fatalf("距首次触发仅 30 秒也应注册成功：%v", err)
		}
		time.Sleep(30 * time.Second)
		synctest.Wait()
		j, err := s.FindJob("id")
		if err != nil {
			t.Fatal(err)
		}
		first := registered.Add(30 * time.Second)
		if calls.Load() != 1 || !j.LastRun.Equal(first) || !j.NextRun.Equal(first.Add(time.Minute)) {
			t.Fatalf("首次应在 %v 派发，第二次应在一分钟后，实际状态：%+v", first, j)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatalf("第二个整分钟应累计派发两次，实际 %d 次", calls.Load())
		}
	})
}

func TestPanicIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		var calls atomic.Int32
		if err := s.Every("checker", "", time.Now(), time.Minute, func() { t.Error("异常检查器仍触发派发") }, func(*Job) bool { panic("checker") }); err != nil {
			t.Fatal(err)
		}
		if err := s.Every("callback", "", time.Now(), time.Minute, func() { calls.Add(1); panic("callback") }, UntilFreq(1)); err != nil {
			t.Fatal(err)
		}
		s.Start()
		defer s.Stop()
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if len(s.Jobs()) != 0 || calls.Load() != 1 {
			t.Errorf("异常隔离后仍有 %d 个任务，派发 %d 次", len(s.Jobs()), calls.Load())
		}
	})
}

func TestStoppedCheckerCannotDispatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		entered := make(chan struct{})
		release := make(chan struct{}, 1)
		defer close(release)
		err := s.Every("id", "", time.Now().Add(-time.Minute), time.Minute, func() {
			t.Error("旧任务仍被派发")
		}, func(*Job) bool {
			close(entered)
			<-release
			return false
		}, func(*Job) bool {
			t.Error("重启后仍调用旧任务的下一个 Checker")
			return false
		})
		if err != nil {
			t.Fatal(err)
		}
		s.Start()
		<-entered
		s.Stop()
		if _, err := s.FindJob("id"); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("停止后仍能查到检查中的任务：%v", err)
		}
		var calls atomic.Int32
		if err := s.RunAt("id", "新任务", time.Now().Add(time.Minute), func() { calls.Add(1) }); err != nil {
			t.Fatalf("停止后无法复用 ID：%v", err)
		}
		s.Stop() // 重复停止，下一轮新登记的任务仍待派发。
		s.Start()
		s.Start()
		defer s.Stop()
		release <- struct{}{}
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls.Load() != 1 || len(s.Jobs()) != 0 {
			t.Errorf("新任务派发 %d 次，剩余任务 %v", calls.Load(), s.Jobs())
		}
	})
}

func TestCronRescheduleFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		// 模拟续排计算返回当前时刻，验证调度循环对错误结果的保护。
		var calls atomic.Int32
		err := s.addJob(&scheduledJob{Job: Job{ID: "id", NextRun: time.Now(), Func: func() { calls.Add(1) }}, nextCron: func(now time.Time) time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		s.Start()
		defer s.Stop()
		synctest.Wait()
		if calls.Load() != 1 || len(s.Jobs()) != 0 {
			t.Fatalf("失效规则派发 %d 次，留下 %d 个任务", calls.Load(), len(s.Jobs()))
		}
		if err := s.RunAt("id", "", time.Now().Add(time.Hour), func() {}); err != nil {
			t.Fatalf("无法复用已结束任务的 ID: %v", err)
		}
	})
}

func TestMinimumRepeatSpacing(t *testing.T) {
	for _, kind := range []string{"Every", "Crontab"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := New()
				start := time.Now()
				fired := make(chan time.Time, 3)
				fn := func() { fired <- time.Now() }
				var err error
				if kind == "Every" {
					err = s.Every("id", "", start, time.Minute, fn)
				} else {
					err = s.Crontab("id", "", "* * * * *", fn)
				}
				if err != nil {
					t.Fatal(err)
				}
				// 第一轮到期后延迟 30 秒启动，从实际派发时刻验证间隔。
				time.Sleep(90 * time.Second)
				s.Start()
				defer s.Stop()
				synctest.Wait()
				if len(fired) != 1 {
					t.Fatalf("首次派发次数 %d", len(fired))
				}
				first := <-fired
				time.Sleep(time.Minute - time.Nanosecond)
				synctest.Wait()
				if len(fired) != 0 {
					t.Fatal("未满 60 秒就再次派发")
				}
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				if len(fired) != 1 {
					t.Fatal("满 60 秒时未派发")
				}
				if gap := (<-fired).Sub(first); gap != time.Minute {
					t.Errorf("两次派发间隔 %v", gap)
				}
			})
		})
	}
}

func TestConcurrentJobSnapshots(t *testing.T) {
	s := New()
	fired := make(chan struct{}, 1)
	if err := s.Every("id", "", time.Now().Add(-time.Minute), time.Minute, func() { fired <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	s.Start()
	defer s.Stop()
	for range 500 {
		j, err := s.FindJob("id")
		if err != nil {
			t.Fatal(err)
		}
		if (j.ExecutionCount == 0) != j.LastRun.IsZero() {
			t.Fatalf("任务计数与派发时间不一致：%+v", j)
		}
	}
	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Fatal("任务未被派发")
	}
}

func TestSlowCheckerDoesNotBlockOtherJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		wait := make(chan struct{})
		defer close(wait)
		if err := s.Every("slow", "", time.Now().Add(-time.Minute), time.Minute, func() { t.Error("任务应被终止") }, func(*Job) bool { <-wait; return true }); err != nil {
			t.Fatal(err)
		}
		var calls atomic.Int32
		if err := s.RunAt("other", "", time.Now().Add(time.Second), func() { calls.Add(1) }); err != nil {
			t.Fatal(err)
		}
		s.Start()
		defer s.Stop()
		time.Sleep(time.Second)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("其他任务被慢 Checker 阻塞")
		}
	})
}
