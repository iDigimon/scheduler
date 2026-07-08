package scheduler

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestRunAt_3Seconds 测试3秒后执行任务的功能
func TestRunAt_3Seconds(t *testing.T) {
	// 创建调度器
	s := New()
	s.Start()
	defer s.Stop()

	// 创建WaitGroup以确保任务执行完成
	var wg sync.WaitGroup
	wg.Add(1)

	// 标记任务是否执行
	taskExecuted := false
	executionTime := time.Time{}

	// 设置3秒后执行的时间点
	executeTime := time.Now().Add(3 * time.Second)

	// 创建任务
	err := s.RunAt("3s-task", "3秒后执行任务", executeTime, func() {
		defer wg.Done()
		taskExecuted = true
		executionTime = time.Now()
		fmt.Printf("✅ 任务在预期时间执行: %s\n", executionTime.Format("15:04:05.000"))
	})

	if err != nil {
		t.Fatalf("❌ 创建3秒后执行的任务失败: %v", err)
	}

	// 等待3秒半，确保任务有足够的时间执行
	t.Logf("等待3.5秒让任务执行...")
	time.Sleep(3500 * time.Millisecond)

	// 等待任务执行完成（如果尚未完成）
	wg.Wait()

	// 验证任务执行状态
	if !taskExecuted {
		t.Errorf("❌ 测试失败: 3秒后执行的任务未执行")
	} else {
		// 检查执行时间是否在预期范围内（允许100毫秒的误差）
		timeDiff := executionTime.Sub(executeTime).Abs()
		if timeDiff > 100*time.Millisecond {
			t.Errorf("❌ 测试失败: 任务执行时间与预期偏差过大 (偏差: %v)", timeDiff)
		} else {
			t.Logf("✅ 测试通过: 3秒后执行的任务成功执行，执行时间偏差在允许范围内 (%v)", timeDiff)
		}
	}
}

// TestRunAt_MultipleTasks 测试同时添加多个3秒后执行的任务
func TestRunAt_MultipleTasks(t *testing.T) {
	// 创建调度器
	s := New()
	s.Start()
	defer s.Stop()

	// 创建WaitGroup以确保所有任务执行完成
	var wg sync.WaitGroup
	taskCount := 3
	wg.Add(taskCount)

	// 用于跟踪每个任务的执行状态
	executedTasks := make(map[string]bool, taskCount)
	var mu sync.Mutex // 保护共享变量

	// 计算执行时间（3秒后）
	executeTime := time.Now().Add(3 * time.Second)

	// 添加多个任务
	for i := 1; i <= taskCount; i++ {
		taskID := fmt.Sprintf("multi-task-%d", i)
		taskName := fmt.Sprintf("第%d个3秒后执行任务", i)
		currentTaskID := taskID

		err := s.RunAt(taskID, taskName, executeTime, func() {
			defer wg.Done()
			mu.Lock()
			executedTasks[currentTaskID] = true
			mu.Unlock()
			fmt.Printf("✅ 多任务测试 - 任务 %s 执行完成\n", currentTaskID)
		})

		if err != nil {
			t.Fatalf("❌ 创建多任务测试中的任务 %d 失败: %v", i, err)
		}
	}

	// 等待4秒，确保所有任务有足够的时间执行
	t.Logf("等待4秒让所有任务执行...")
	time.Sleep(4 * time.Second)

	// 等待所有任务执行完成
	wg.Wait()

	// 验证所有任务都已执行
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i <= taskCount; i++ {
		taskID := fmt.Sprintf("multi-task-%d", i)
		if !executedTasks[taskID] {
			t.Errorf("❌ 测试失败: 多任务测试中的任务 %s 未执行", taskID)
		}
	}

	// 如果所有任务都执行了，记录成功信息
	if len(executedTasks) == taskCount {
		t.Logf("✅ 测试通过: 所有%d个3秒后执行的任务都成功执行", taskCount)
	}
}
