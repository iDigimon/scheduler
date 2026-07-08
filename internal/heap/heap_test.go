package heap

import (
	"slices"
	"testing"
	"time"
)

// 本文件针对 internal/heap/min_heap.go 中真实导出的 Heap[T] API 编写，
// 公开 API 为：New / Len / Top / Push / Pop / Remove / Update / FindIndex。
//
// 注意空堆语义：Top()/Pop() 在空堆上返回零值且不会 panic；
// Remove 越界返回零值；Update 越界为 no-op。
// 所有断言均以 min_heap.go 的实际行为为准。
//
// 分工说明：
//   - heap_test.go         —— Heap[T] 核心行为与边界（本文件）
//   - int_heap_test.go     —— Heap[int] 整数特化场景与基准
//   - ordered_heap_test.go —— Heap[T] 在有序类型（string 等）上的用法与基准

// intLess 是一个最简单的 int 最小堆 less 函数。
func intLess(a, b int) bool { return a < b }

// drainAll 反复 Pop 直到堆为空，返回弹出顺序。
func drainAll[T any](h *Heap[T]) []T {
	out := make([]T, 0, h.Len())
	for h.Len() > 0 {
		out = append(out, h.Pop())
	}
	return out
}

func TestNewAndLen(t *testing.T) {
	h := New[int](intLess)
	if h.Len() != 0 {
		t.Errorf("新建堆长度应为 0，实际 %d", h.Len())
	}
}

func TestPushAndTop(t *testing.T) {
	h := New[int](intLess)
	h.Push(5)
	h.Push(3)
	h.Push(7)
	h.Push(1)

	if h.Len() != 4 {
		t.Fatalf("Push 4 个元素后长度应为 4，实际 %d", h.Len())
	}

	// 最小堆：堆顶应为最小值 1。
	if top := h.Top(); top != 1 {
		t.Errorf("Top() = %d, 期望 1", top)
	}

	// Top 不应移除元素。
	if h.Len() != 4 {
		t.Errorf("Top() 不应改变长度，实际 %d", h.Len())
	}
}

// TestTopOnEmpty 空堆 Top 返回零值，不 panic。
func TestTopOnEmpty(t *testing.T) {
	h := New[int](intLess)
	if top := h.Top(); top != 0 {
		t.Errorf("空堆 Top() = %d, 期望零值 0", top)
	}
}

// TestPopOnEmpty 空堆 Pop 返回零值，不 panic。
func TestPopOnEmpty(t *testing.T) {
	h := New[int](intLess)
	if v := h.Pop(); v != 0 {
		t.Errorf("空堆 Pop() = %d, 期望零值 0", v)
	}
	if h.Len() != 0 {
		t.Errorf("空堆 Pop 后长度应仍为 0，实际 %d", h.Len())
	}
}

// TestMaxHeap 用反向 less 构造最大堆，Pop 应得到降序序列。
func TestMaxHeap(t *testing.T) {
	h := New[int](func(a, b int) bool { return a > b })
	for _, v := range []int{5, 3, 7, 1, 9, 2} {
		h.Push(v)
	}

	want := []int{9, 7, 5, 3, 2, 1}
	var got []int
	for h.Len() > 0 {
		got = append(got, h.Pop())
	}
	if !slices.Equal(got, want) {
		t.Errorf("最大堆 Pop 序列 = %v, 期望 %v", got, want)
	}
}

func TestRemove(t *testing.T) {
	t.Run("删除中间元素后堆序正确", func(t *testing.T) {
		h := New[int](intLess)
		for _, v := range []int{5, 3, 7, 1, 9, 2} {
			h.Push(v)
		}

		idx := h.FindIndex(func(v int) bool { return v == 7 })
		if idx < 0 {
			t.Fatalf("FindIndex 未找到 7")
		}
		got := h.Remove(idx)
		if got != 7 {
			t.Errorf("Remove 返回 %d, 期望 7", got)
		}
		if h.Len() != 5 {
			t.Errorf("Remove 后长度应为 5，实际 %d", h.Len())
		}

		out := drainAll(h)
		if !slices.Equal(out, []int{1, 2, 3, 5, 9}) {
			t.Errorf("删除后 Pop 序列 = %v, 期望 [1 2 3 5 9]", out)
		}
	})

	t.Run("删除末尾元素", func(t *testing.T) {
		h := New[int](intLess)
		for _, v := range []int{5, 3, 7, 1} {
			h.Push(v)
		}
		last := h.Len() - 1
		h.Remove(last)
		// 末尾删除走的是 Remove 的 fast path（直接截断）。
		if h.Len() != 3 {
			t.Errorf("删除末尾后长度应为 3，实际 %d", h.Len())
		}
		// 删除后仍应能正常弹出升序序列。
		out := drainAll(h)
		if !slices.IsSorted(out) {
			t.Errorf("删除末尾后序列非升序：%v", out)
		}
	})

	t.Run("删除堆顶等价于 Pop", func(t *testing.T) {
		h := New[int](intLess)
		for _, v := range []int{5, 3, 7, 1} {
			h.Push(v)
		}
		if got := h.Remove(0); got != 1 {
			t.Errorf("Remove(0) = %d, 期望 1", got)
		}
		if top := h.Top(); top != 3 {
			t.Errorf("删除堆顶后 Top() = %d, 期望 3", top)
		}
	})

	t.Run("越界 Remove 返回零值且不改变长度", func(t *testing.T) {
		h := New[int](intLess)
		h.Push(1)
		h.Push(2)

		if got := h.Remove(-1); got != 0 {
			t.Errorf("Remove(-1) 应返回零值，实际 %d", got)
		}
		if got := h.Remove(h.Len()); got != 0 {
			t.Errorf("Remove(len) 应返回零值，实际 %d", got)
		}
		if h.Len() != 2 {
			t.Errorf("越界 Remove 后长度应仍为 2，实际 %d", h.Len())
		}
	})
}

// TestRemoveMiddleReheapify 刻意构造 Remove 的 up / down 两条分支。
// min_heap.go 中 Remove 把末尾元素搬到被删位置，再按需上浮或下沉。
func TestRemoveMiddleReheapify(t *testing.T) {
	t.Run("末尾元素更小需要上浮", func(t *testing.T) {
		// 构造一个最小堆，删除中间较大元素后，搬过去的末尾元素很小，
		// 需要向上调整。
		h := New[int](intLess)
		for _, v := range []int{10, 20, 11, 30, 40, 12, 13, 1} {
			h.Push(v)
		}

		// 删除值为 20 的元素（中间位置）。
		idx := h.FindIndex(func(v int) bool { return v == 20 })
		if idx < 0 {
			t.Fatalf("未找到 20")
		}
		h.Remove(idx)

		out := drainAll(h)
		if !slices.IsSorted(out) {
			t.Errorf("Remove 后序列非升序：%v", out)
		}
		if len(out) != 7 {
			t.Errorf("应剩 7 个元素，实际 %d", len(out))
		}
	})

	t.Run("末尾元素更大需要下沉", func(t *testing.T) {
		h := New[int](intLess)
		// 这里插入顺序保证：删除某中间元素后，搬过去的末尾值偏大，需下沉。
		for _, v := range []int{1, 5, 2, 8, 9, 3, 4, 100} {
			h.Push(v)
		}

		idx := h.FindIndex(func(v int) bool { return v == 5 })
		if idx < 0 {
			t.Fatalf("未找到 5")
		}
		h.Remove(idx)

		out := drainAll(h)
		if !slices.IsSorted(out) {
			t.Errorf("Remove 后序列非升序：%v", out)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("减小值触发上浮", func(t *testing.T) {
		h := New[int](intLess)
		for _, v := range []int{5, 3, 7, 1, 9, 2} {
			h.Push(v)
		}

		// 把 9 改成 0，应上浮到堆顶。
		idx := h.FindIndex(func(v int) bool { return v == 9 })
		if idx < 0 {
			t.Fatalf("未找到 9")
		}
		h.Update(idx, 0)

		if top := h.Top(); top != 0 {
			t.Errorf("Update 后 Top() = %d, 期望 0", top)
		}
		out := drainAll(h)
		if !slices.IsSorted(out) {
			t.Errorf("Update 后序列非升序：%v", out)
		}
	})

	t.Run("增大值触发下沉", func(t *testing.T) {
		h := New[int](intLess)
		for _, v := range []int{5, 3, 7, 1, 9, 2} {
			h.Push(v)
		}

		// 把堆顶 1 改成 100，应下沉。
		h.Update(0, 100)

		out := drainAll(h)
		if !slices.IsSorted(out) {
			t.Errorf("Update 后序列非升序：%v", out)
		}
	})

	t.Run("越界 Update 为 no-op", func(t *testing.T) {
		h := New[int](intLess)
		h.Push(1)
		h.Push(2)

		// 越界调用不应 panic，也不改变堆内容。
		h.Update(-1, 999)
		h.Update(h.Len(), 999)

		out := drainAll(h)
		if !slices.IsSorted(out) {
			t.Errorf("越界 Update 后序列非升序：%v", out)
		}
	})
}

func TestFindIndex(t *testing.T) {
	h := New[int](intLess)
	for _, v := range []int{5, 3, 7, 1, 9, 2} {
		h.Push(v)
	}

	t.Run("命中", func(t *testing.T) {
		idx := h.FindIndex(func(v int) bool { return v == 7 })
		if idx < 0 {
			t.Errorf("FindIndex 未找到存在的值 7")
		}
	})

	t.Run("未命中返回 -1", func(t *testing.T) {
		idx := h.FindIndex(func(v int) bool { return v == 42 })
		if idx != -1 {
			t.Errorf("FindIndex 未命中应返回 -1，实际 %d", idx)
		}
	})
}

// item 用于验证泛型对结构体 + 复合 less 的支持。
type item struct {
	priority int
	name     string
}

func TestHeapWithStruct(t *testing.T) {
	// 复合 less：先按 priority 升序，priority 相同按 name 升序。
	less := func(a, b item) bool {
		if a.priority != b.priority {
			return a.priority < b.priority
		}
		return a.name < b.name
	}
	h := New[item](less)
	items := []item{
		{2, "b"},
		{1, "z"},
		{1, "a"},
		{3, "a"},
		{1, "m"},
	}
	for _, it := range items {
		h.Push(it)
	}

	want := []item{
		{1, "a"},
		{1, "m"},
		{1, "z"},
		{2, "b"},
		{3, "a"},
	}
	out := drainAll(h)
	if !slices.EqualFunc(out, want, func(a, b item) bool {
		return a.priority == b.priority && a.name == b.name
	}) {
		t.Errorf("结构体堆 Pop 序列 = %v, 期望 %v", out, want)
	}
}

// job 镜像 scheduler.Job 中调度器实际使用的关键字段，
// 用于在测试包内（避免循环导入 scheduler 包）验证 Heap[*T]
// 与 scheduler.go 中「按 NextRun 排序的最小堆」用法等价。
type job struct {
	ID      string
	NextRun time.Time
}

// TestJobHeapMirrorsScheduler 覆盖 scheduler.go 实际依赖的全部方法组合：
// New(用 NextRun.Before 构造 less) / Push / Top / Pop / FindIndex / Remove / Update。
func TestJobHeapMirrorsScheduler(t *testing.T) {
	h := New[*job](func(i, j *job) bool {
		return i.NextRun.Before(j.NextRun)
	})

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jobs := []*job{
		{ID: "c", NextRun: base.Add(3 * time.Second)},
		{ID: "a", NextRun: base.Add(1 * time.Second)},
		{ID: "b", NextRun: base.Add(2 * time.Second)},
		{ID: "d", NextRun: base.Add(4 * time.Second)},
	}
	for _, j := range jobs {
		h.Push(j)
	}

	// 堆顶应为最早执行的 a。
	if top := h.Top(); top == nil || top.ID != "a" {
		var gotID string
		if top != nil {
			gotID = top.ID
		}
		t.Errorf("Top() = %q, 期望 \"a\"", gotID)
	}

	// 按 ID 查找并删除 b（模拟 scheduler.Remove）。
	idx := h.FindIndex(func(j *job) bool { return j.ID == "b" })
	if idx < 0 {
		t.Fatalf("FindIndex 未找到 b")
	}
	removed := h.Remove(idx)
	if removed == nil || removed.ID != "b" {
		t.Errorf("Remove 返回 %v, 期望 b", removed)
	}
	if h.Len() != 3 {
		t.Errorf("Remove 后长度应为 3，实际 %d", h.Len())
	}

	// 更新 a 的执行时间到最晚，应触发下沉，堆顶变化。
	idxA := h.FindIndex(func(j *job) bool { return j.ID == "a" })
	if idxA < 0 {
		t.Fatalf("FindIndex 未找到 a")
	}
	h.Update(idxA, &job{ID: "a", NextRun: base.Add(99 * time.Second)})
	if top := h.Top(); top == nil || top.ID != "c" {
		var gotID string
		if top != nil {
			gotID = top.ID
		}
		t.Errorf("Update 后 Top() = %q, 期望 \"c\"", gotID)
	}

	// Pop 应按时间升序，a 因被推迟到最后才弹出。
	var order []string
	for h.Len() > 0 {
		j := h.Pop()
		if j == nil {
			t.Fatal("Pop 返回 nil")
		}
		order = append(order, j.ID)
	}
	if want := []string{"c", "d", "a"}; !slices.Equal(order, want) {
		t.Errorf("job 堆 Pop 序列 = %v, 期望 %v", order, want)
	}
}
