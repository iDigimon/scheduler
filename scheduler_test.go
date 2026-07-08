package scheduler

import (
	"fmt"
	"testing"
	"time"
)

// 测试JobChecker功能
func TestJobChecker(t *testing.T) {
	// 创建调度器
	s := New()
	s.Start()
	defer s.Stop()

	// 创建一个计数器
	count := 0

	// 创建截止时间（10秒后）
	endTime := time.Now().Add(10 * time.Second)

	// 创建使用JobChecker的任务
	// 这个任务将执行3次后停止
	err := s.Every("test-job", "测试任务", time.Now(), time.Minute, func() {
		count++
		fmt.Printf("任务执行次数: %d\n", count)
	}, UntilFreq(3)) // 限制执行3次

	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	// 等待足够的时间让任务执行
	time.Sleep(8 * time.Second)

	// 验证任务执行次数不超过3次
	if count > 3 {
		t.Errorf("任务执行次数超过预期: %d, 应该不超过3次", count)
	}

	// 创建一个带截止时间的任务
	count2 := 0
	endTimeChecker := UntilTime(endTime)

	err = s.Crontab("test-cron", "Cron任务", "* * * * *", func() {
		count2++
		fmt.Printf("Cron任务执行次数: %d\n", count2)
	}, endTimeChecker) // 使用时间截止检查器

	if err != nil {
		t.Fatalf("创建Cron任务失败: %v", err)
	}

	// 再等待一段时间，确保截止时间已过
	time.Sleep(5 * time.Second)

	// 验证任务执行次数符合预期
	fmt.Printf("测试完成: 普通任务执行了%d次, Cron任务执行了%d次\n", count, count2)
}

// 测试自定义JobChecker
func TestCustomJobChecker(t *testing.T) {
	// 创建调度器
	s := New()
	s.Start()
	defer s.Stop()

	// 创建一个自定义的JobChecker，当当前时间的分钟数为偶数时停止任务
	customChecker := func(job *Job) bool {
		currentMinute := time.Now().Minute()
		return currentMinute%2 == 0
	}

	// 创建任务
	err := s.Every("custom-job", "自定义检查器任务", time.Now(), time.Minute, func() {
		fmt.Printf("自定义任务执行时间: %s\n", time.Now().Format("15:04:05"))
	}, customChecker)

	if err != nil {
		t.Fatalf("创建任务失败: %v", err)
	}

	// 等待足够的时间让任务有机会执行
	time.Sleep(2 * time.Minute)
}