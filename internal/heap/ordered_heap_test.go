package heap

import (
	"cmp"
	"slices"
	"testing"
)

// 本文件覆盖 Heap[T] 在有序类型（string 等）及自定义 less 上的用法，
// 并集中放置所有基准测试。
// 文件名 ordered_heap_test.go 保留不变；历史上它曾含一套独立的
// orderedHeap[T cmp.Ordered] 参考实现，现已改为只测真实的 Heap[T]。

// TestOrderedHeapStringSort 验证 Heap[string] 按 < 排序。
func TestOrderedHeapStringSort(t *testing.T) {
	h := New[string](func(a, b string) bool { return a < b })
	for _, s := range []string{"dog", "cat", "bird", "ant"} {
		h.Push(s)
	}
	if top := h.Top(); top != "ant" {
		t.Errorf("Top() = %q, 期望 %q", top, "ant")
	}
	out := drainAll(h)
	want := []string{"ant", "bird", "cat", "dog"}
	if !slices.Equal(out, want) {
		t.Errorf("字符串堆 Pop 序列 = %v, 期望 %v", out, want)
	}
}

// TestOrderedHeapCmpCompare 用 cmp.Compare 构造 less，
// 验证与调度器一样可基于标准库比较器使用 Heap[T]。
func TestOrderedHeapCmpCompare(t *testing.T) {
	less := func(a, b int) bool { return cmp.Compare(a, b) < 0 }
	h := New[int](less)
	for _, v := range []int{5, 2, 8, 1, 9, 3, 7} {
		h.Push(v)
	}
	out := drainAll(h)
	if !slices.IsSorted(out) {
		t.Errorf("cmp.Compare 构造的堆 Pop 序列非升序：%v", out)
	}
	if !slices.Equal(out, []int{1, 2, 3, 5, 7, 8, 9}) {
		t.Errorf("Pop 序列 = %v, 期望 [1 2 3 5 7 8 9]", out)
	}
}

// TestOrderedHeapDuplicates 验证有序类型下重复元素的处理。
func TestOrderedHeapDuplicates(t *testing.T) {
	h := New[string](func(a, b string) bool { return a < b })
	for _, s := range []string{"b", "a", "b", "a", "c"} {
		h.Push(s)
	}
	if h.Len() != 5 {
		t.Fatalf("含重复元素长度应为 5，实际 %d", h.Len())
	}
	out := drainAll(h)
	if !slices.Equal(out, []string{"a", "a", "b", "b", "c"}) {
		t.Errorf("含重复元素 Pop 序列 = %v, 期望 [a a b b c]", out)
	}
}

// --- 基准测试 ---
// benchN 是所有基准共用的堆规模常量，统一定义于此。
// Benchmark*Push/Pop/PushPopMix 命名跨 heap_test.go 外的文件保持一致，
// 便于横向对比不同元素类型 / less 函数的开销。

const benchN = 10000

// BenchmarkOrderedHeapPush 单独压测 Heap[int]（cmp.Compare 构造 less）的 Push。
func BenchmarkOrderedHeapPush(b *testing.B) {
	less := func(a, b int) bool { return cmp.Compare(a, b) < 0 }
	h := New[int](less)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Push(i)
		if h.Len() >= benchN {
			h.Pop()
		}
	}
}

// BenchmarkOrderedHeapPop 单独压测 Pop。
func BenchmarkOrderedHeapPop(b *testing.B) {
	less := func(a, b int) bool { return cmp.Compare(a, b) < 0 }
	h := New[int](less)
	for i := 0; i < benchN; i++ {
		h.Push(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Pop()
		if h.Len() == 0 {
			for j := 0; j < benchN; j++ {
				h.Push(j)
			}
		}
	}
}

// BenchmarkOrderedHeapPushPopMix 混合 Push/Pop。
func BenchmarkOrderedHeapPushPopMix(b *testing.B) {
	less := func(a, b int) bool { return cmp.Compare(a, b) < 0 }
	h := New[int](less)
	for i := 0; i < benchN; i++ {
		h.Push(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Pop()
		h.Push(i + benchN)
	}
}
