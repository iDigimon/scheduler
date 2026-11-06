package scheduler

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"
)

// TestRunAt_3Seconds 验证运行中插入更早的任务会唤醒定时器。
func TestRunAt_3Seconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		s.Start()
		defer s.Stop()
		start := time.Now()
		if err := s.RunAt("later", "", start.Add(time.Hour), func() { t.Error("已移除任务仍被派发") }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		at := start.Add(3 * time.Second)
		fired := make(chan time.Time, 1)
		if err := s.RunAt("once", "", at, func() { fired <- time.Now() }); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		select {
		case got := <-fired:
			if !got.Equal(at) {
				t.Errorf("实际执行时间 %v，期望 %v", got, at)
			}
		default:
			t.Fatal("一次性任务未准时执行")
		}
		if err := s.Remove("later"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		if len(s.Jobs()) != 0 {
			t.Fatal("仍有未清理的任务")
		}
	})
}

func TestRunAt_MultipleTasks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := New()
		at := time.Now().Add(3 * time.Second)
		fired := make(chan int, 3)
		for i := range 3 {
			if err := s.RunAt(fmt.Sprint(i), "", at, func() { fired <- i }); err != nil {
				t.Fatal(err)
			}
		}
		s.Start()
		defer s.Stop()
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if len(fired) != 3 {
			t.Fatalf("同时到期的任务只派发了 %d 个", len(fired))
		}
		var seen [3]bool
		for range seen {
			id := <-fired
			if id < 0 || id >= len(seen) || seen[id] {
				t.Fatalf("任务 ID 错误或重复派发：%d", id)
			}
			seen[id] = true
		}
	})
}
