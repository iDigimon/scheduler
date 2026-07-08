package heap

import (
	"slices"
	"testing"
)

// 本文件聚焦 Heap[int] 的整数场景：排序语义、TopK 用法、大规模数据，
// 以及针对 Heap[int] 的基准测试。
// 文件名 int_heap_test.go 保留不变；历史上它曾含一套独立的 intHeap 参考实现，
// 现已改为只测真实的 Heap[T]。

// TestIntHeapSort 验证 Heap[int] 连续 Pop 得到升序序列（含重复值）。
func TestIntHeapSort(t *testing.T) {
	h := New[int](intLess)
	for _, v := range []int{5, 2, 8, 1, 9, 3, 7, 2, 7} {
		h.Push(v)
	}

	var got []int
	for h.Len() > 0 {
		got = append(got, h.Pop())
	}

	want := []int{1, 2, 2, 3, 5, 7, 7, 8, 9}
	if !slices.Equal(got, want) {
		t.Errorf("Pop 序列 = %v, 期望 %v", got, want)
	}
}

// TestIntHeapTopK 用 Heap[int] 维护前 K 大元素（经典最小堆 TopK）。
// 当新元素大于堆顶时，弹出堆顶并压入新元素。
func TestIntHeapTopK(t *testing.T) {
	data := []int{7, 2, 9, 1, 5, 8, 3, 6, 4, 10}
	const k = 3

	h := New[int](intLess)
	for _, v := range data[:k] {
		h.Push(v)
	}

	for _, v := range data[k:] {
		if v > h.Top() {
			h.Pop()
			h.Push(v)
		}
	}

	var got []int
	for h.Len() > 0 {
		got = append(got, h.Pop())
	}

	want := []int{8, 9, 10}
	if !slices.Equal(got, want) {
		t.Errorf("TopK 结果 = %v, 期望 %v", got, want)
	}
}

// TestIntHeapLargeScale 大规模逆序插入后 Pop 必须全程严格升序。
func TestIntHeapLargeScale(t *testing.T) {
	const n = 1000
	h := New[int](intLess)

	for i := n; i > 0; i-- {
		h.Push(i)
	}

	prev := -1
	count := 0
	for h.Len() > 0 {
		v := h.Pop()
		if v <= prev {
			t.Errorf("Pop 得到 %d，未严格大于前一个 %d", v, prev)
		}
		prev = v
		count++
	}
	if count != n {
		t.Errorf("共弹出 %d 个，期望 %d", count, n)
	}
	if prev != n {
		t.Errorf("最后一个弹出值 = %d, 期望 %d", prev, n)
	}
}

// --- 基准测试：针对真实的 Heap[int]。
// benchN 在 ordered_heap_test.go 中统一定义，三类基准跨文件共享。 ---

// BenchmarkIntHeapPush 单独压测 Push（控制堆规模避免无限增长）。
func BenchmarkIntHeapPush(b *testing.B) {
	h := New[int](intLess)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Push(i)
		if h.Len() >= benchN {
			h.Pop()
		}
	}
}

// BenchmarkIntHeapPop 单独压测 Pop（保持堆不空）。
func BenchmarkIntHeapPop(b *testing.B) {
	h := New[int](intLess)
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

// BenchmarkIntHeapPushPopMix 混合 Push/Pop，模拟调度器的工作负载。
func BenchmarkIntHeapPushPopMix(b *testing.B) {
	h := New[int](intLess)
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
