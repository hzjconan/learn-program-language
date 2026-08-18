package genx

import "testing"

// filterNoPrealloc 是对照组：不预分配，让 append 自己扩容
func filterNoPrealloc[T any](s []T, keep func(T) bool) []T {
	var out []T
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var sinkSlice []int

func BenchmarkFilter_PreAllocate_100PercentHit(b *testing.B) {
	data := makeData(10000, 1) // 全命中

	logCapWaste(b, Filter(data, isHit))

	for b.Loop() {
		sinkSlice = Filter(data, isHit)
	}
}

func BenchmarkFilter_NoPreAllocate_100PercentHit(b *testing.B) {
	data := makeData(10000, 1) // 全命中

	logCapWaste(b, filterNoPrealloc(data, isHit))

	for b.Loop() {
		sinkSlice = filterNoPrealloc(data, isHit)
	}
}

func BenchmarkFilter_Prealloc_50pct(b *testing.B) {
	data := makeData(10000, 2) // 一半

	logCapWaste(b, Filter(data, isHit))

	for b.Loop() {
		sinkSlice = Filter(data, isHit)
	}
}

func BenchmarkFilter_NoPreAllocate_50PercentHit(b *testing.B) {
	data := makeData(10000, 2) // 一半

	logCapWaste(b, filterNoPrealloc(data, isHit))

	for b.Loop() {
		sinkSlice = filterNoPrealloc(data, isHit)
	}
}

func BenchmarkFilter_Prealloc_1pct(b *testing.B) {
	data := makeData(10000, 10000) // 只有 1 个

	logCapWaste(b, Filter(data, isHit))

	for b.Loop() {
		sinkSlice = Filter(data, isHit)
	}
}

func BenchmarkFilter_NoPreAllocate_1PercentHit(b *testing.B) {
	data := makeData(10000, 10000) // 只有 1 个

	logCapWaste(b, filterNoPrealloc(data, isHit))

	for b.Loop() {
		sinkSlice = filterNoPrealloc(data, isHit)
	}
}

// makeData 返回长度为 n 的切片，每隔 hitEvery 个元素放一个命中标记。
// hitEvery=1 → 100% 命中；2 → 50%；n → 1/n
func makeData(n, hitEvery int) []int {
	s := make([]int, n) // ⭐ len=n，不是 cap=n
	for i := range s {
		if i%hitEvery == 0 {
			s[i] = 1
		}
	}
	return s
}

func isHit(v int) bool { return v == 1 }

func logCapWaste(b *testing.B, got []int) {
	b.Helper() // 和 t.Helper 一样，让失败行号指向调用处（讲义 §4）

	// 参考写法：报告结果切片的容量浪费。
	//
	// 三个要点：
	//  1. 参数是 *testing.B，所以是 b.Logf 不是 t.Logf
	//  2. ⭐ 必须放在 for b.Loop() 【外面】—— 放里面会打印几万遍
	//  3. b.Logf 【不加 -v 也会显示】，挂在 "--- BENCH:" 下面。
	//     这和 t.Logf 不同：测试通过时 t.Logf 必须加 -v 才看得到。
	//     （benchmark 本来就是拿来看输出的，所以日志默认可见。）
	b.Logf("len=%d cap=%d 白占 %d 个元素（%.1f%%）",
		len(got), cap(got), cap(got)-len(got),
		float64(cap(got)-len(got))/float64(cap(got))*100)
}

// 跑出来的结果如下。
// 结果解读：
// 1. 预分配切片容量在高命中率的时候，避免扩容，提高性能。
// 2. 相反，在低命中率的时候严重浪费内存
// 需不需要修改：作为练习，暂时不要。如果实际工程上碰到这类问题，先要了解实际的应用场景，
// 如果本身就应该是期望高命中的，那不用改，反之，要考虑是不是真的应该在一个巨大的切片里面
// 去过滤出很少几个数据，有可能调用方本身设计就有问题。建议在函数文档里写明这个问题，让调用方
// 去决定： cap 可能远大于 len，需要长期持有请自行 slices.Clone

/**
go test -bench='BenchmarkFilter' -benchmem -run='^$' ./internal/genx/
goos: darwin
goarch: arm64
pkg: github.com/hzjconan/learn-program-language/go/internal/genx
cpu: Apple M1 Pro
BenchmarkFilter_PreAllocate_100PercentHit-10               99322             11179 ns/op        81920 B/op          1 allocs/op
--- BENCH: BenchmarkFilter_PreAllocate_100PercentHit-10
    genx_bench_test.go:24: len=10000 cap=10000 白占 0 个元素（0.0%）
BenchmarkFilter_NoPreAllocate_100PercentHit-10             32718             37187 ns/op       357627 B/op         19 allocs/op
--- BENCH: BenchmarkFilter_NoPreAllocate_100PercentHit-10
    genx_bench_test.go:34: len=10000 cap=12288 白占 2288 个元素（18.6%）
BenchmarkFilter_Prealloc_50pct-10                          83074             14882 ns/op        81920 B/op          1 allocs/op
--- BENCH: BenchmarkFilter_Prealloc_50pct-10
    genx_bench_test.go:44: len=5000 cap=10000 白占 5000 个元素（50.0%）
BenchmarkFilter_NoPreAllocate_50PercentHit-10              55102             22971 ns/op          128249 B/op             16 allocs/op
--- BENCH: BenchmarkFilter_NoPreAllocate_50PercentHit-10
    genx_bench_test.go:54: len=5000 cap=5120 白占 120 个元素（2.3%）
BenchmarkFilter_Prealloc_1pct-10                          103041             11618 ns/op           81920 B/op              1 allocs/op
--- BENCH: BenchmarkFilter_Prealloc_1pct-10
    genx_bench_test.go:64: len=1 cap=10000 白占 9999 个元素（100.0%）
BenchmarkFilter_NoPreAllocate_1PercentHit-10              187450              6428 ns/op               8 B/op          1 allocs/op
--- BENCH: BenchmarkFilter_NoPreAllocate_1PercentHit-10
    genx_bench_test.go:74: len=1 cap=1 白占 0 个元素（0.0%）
PASS
ok      github.com/hzjconan/learn-program-language/go/internal/genx     9.057s
*/
