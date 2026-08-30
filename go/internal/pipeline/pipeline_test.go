package pipeline

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// 测试文件是我写的，别改。
// 你自己想补的用例写在 pipeline_extra_test.go 里，函数名用 TestPipeline_Extra_Xxx。

// ---------- 工具 ----------

// assertNoLeak 检查一段代码跑完后 goroutine 数量回到原点。
func assertNoLeak(t *testing.T, fn func()) {
	t.Helper()
	before := runtime.NumGoroutine()
	fn()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutine 泄漏了：调用前 %d 条，等了 2 秒后仍有 %d 条\n"+
		"（取消之后，还卡在 out <- v 上的 goroutine 永远不会退出 —— "+
		"每条 goroutine 都要有【两个】退出路径）",
		before, runtime.NumGoroutine())
}

func drain(ch <-chan int) []int {
	var out []int
	for v := range ch {
		out = append(out, v)
	}
	return out
}

func double(n int) int { return n * 2 }

// slowFunc 返回一个耗时 d 的加工函数，并记录同时在跑的最大数量。
func slowFunc(d time.Duration, maxSeen *atomic.Int64) func(int) int {
	var inFlight atomic.Int64
	return func(n int) int {
		cur := inFlight.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		defer inFlight.Add(-1)
		time.Sleep(d)
		return n * 2
	}
}

// ---------- Source ----------

func TestPipeline_SourceEmits(t *testing.T) {
	got := drain(Source(context.Background(), []int{1, 2, 3}))
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("Source = %v, want [1 2 3]", got)
	}
}

func TestPipeline_SourceEmptyClosesChannel(t *testing.T) {
	ch := Source(context.Background(), nil)
	if ch == nil {
		t.Fatal("返回了 nil channel —— nil channel 读写永远阻塞，下游会卡死")
	}
	if got := drain(ch); got != nil {
		t.Errorf("空输入应该得到空结果，实际 %v", got)
	}
}

// TestPipeline_SourceCancelNoLeak 验证「下游不读了」时 Source 能退出。
func TestPipeline_SourceCancelNoLeak(t *testing.T) {
	assertNoLeak(t, func() {
		for range 20 {
			ctx, cancel := context.WithCancel(context.Background())
			ch := Source(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
			<-ch // 只读一个就不要了
			cancel()
		}
	})
}

// ---------- Stage ----------

func TestPipeline_StageTransforms(t *testing.T) {
	in := Source(context.Background(), []int{1, 2, 3})
	got := drain(Stage(context.Background(), in, double))
	if !slices.Equal(got, []int{2, 4, 6}) {
		t.Errorf("Stage = %v, want [2 4 6]", got)
	}
}

func TestPipeline_StageClosesOutput(t *testing.T) {
	in := Source(context.Background(), []int{1})
	out := Stage(context.Background(), in, double)
	<-out
	select {
	case _, ok := <-out:
		if ok {
			t.Error("out 里还有值？")
		}
	case <-time.After(time.Second):
		t.Error("上游关闭后 Stage 没有关闭 out —— 下游的 for range 会永远卡住")
	}
}

func TestPipeline_StageCancelNoLeak(t *testing.T) {
	assertNoLeak(t, func() {
		for range 20 {
			ctx, cancel := context.WithCancel(context.Background())
			in := Source(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
			out := Stage(ctx, in, double)
			<-out // 只读一个
			cancel()
		}
	})
}

// ---------- Merge ----------

func TestPipeline_MergeCollectsAll(t *testing.T) {
	ctx := context.Background()
	a := Source(ctx, []int{1, 2, 3})
	b := Source(ctx, []int{4, 5})
	c := Source(ctx, []int{6})

	got := drain(Merge(ctx, a, b, c))
	slices.Sort(got) // fan-in 的顺序是随机的
	if !slices.Equal(got, []int{1, 2, 3, 4, 5, 6}) {
		t.Errorf("Merge = %v, want [1 2 3 4 5 6]", got)
	}
}

// TestPipeline_MergeClosesAfterAll 验证「所有输入都关了才关输出」。
//
// ⚠️ 慢的那个【放在前面】—— 早先的版本快的在前，串行遍历时结果也对，
// 那条测试对「串行拼接而不是并行 fan-in」是免疫的（D6 那条「对称场景没有区分能力」）。
func TestPipeline_MergeClosesAfterAll(t *testing.T) {
	ctx := context.Background()
	slow := make(chan int)
	go func() {
		time.Sleep(50 * time.Millisecond)
		slow <- 2
		close(slow)
	}()
	fast := Source(ctx, []int{1})

	got := drain(Merge(ctx, (<-chan int)(slow), fast))
	slices.Sort(got)
	if !slices.Equal(got, []int{1, 2}) {
		t.Errorf("Merge = %v, want [1 2]\n"+
			"（快的那个关了就关 out 的话，慢的那个值会丢）", got)
	}
}

// TestPipeline_MergeIsParallel 抓的是「串行拼接冒充 fan-in」。
//
// 错误实现是一条 goroutine 挨个遍历 ins：
//
//	for _, in := range ins { for v := range in { out <- v } }
//
// 它能通过上面那些测试（结果集是对的），但有两个致命问题：
// 后面的 input 要等前面的排空（没有并行）；前面的 input 卡住时后面全部饿死（队头阻塞）。
// 正确做法是每个 input 一条搬运 goroutine + WaitGroup（D10 §8）。
func TestPipeline_MergeIsParallel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blocked := make(chan int) // 永远不发值也不关闭
	ready := Source(ctx, []int{1, 2, 3})

	out := Merge(ctx, (<-chan int)(blocked), ready) // ⭐ 卡住的放第一个

	got := 0
	deadline := time.After(200 * time.Millisecond)
	for got < 3 {
		select {
		case _, ok := <-out:
			if !ok {
				t.Fatalf("out 提前关闭了，只收到 %d 个", got)
			}
			got++
		case <-deadline:
			t.Fatalf("200ms 内只收到 %d 个，want 3\n"+
				"（第一个 input 永远不发值，但第二个 input 里有 3 个现成的值 ——\n"+
				" 收不到说明 Merge 是【串行遍历】ins，不是并行 fan-in，存在队头阻塞）", got)
		}
	}
}

func TestPipeline_MergeEmpty(t *testing.T) {
	ch := Merge(context.Background())
	if ch == nil {
		t.Fatal("返回了 nil channel")
	}
	if got := drain(ch); got != nil {
		t.Errorf("没有输入时应该立刻关闭，实际拿到 %v", got)
	}
}

func TestPipeline_MergeCancelNoLeak(t *testing.T) {
	assertNoLeak(t, func() {
		for range 20 {
			ctx, cancel := context.WithCancel(context.Background())
			ins := make([]<-chan int, 4)
			for i := range ins {
				ins[i] = Source(ctx, []int{1, 2, 3, 4, 5})
			}
			out := Merge(ctx, ins...)
			<-out
			cancel()
		}
	})
}

// ---------- Run ----------

func TestPipeline_RunHappyPath(t *testing.T) {
	items := make([]int, 50)
	want := make([]int, 50)
	for i := range items {
		items[i] = i
		want[i] = i * 2
	}

	got, err := Run(context.Background(), items, 4, double)
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Run 结果不对（要升序排好 —— 并发完成顺序是随机的）\ngot  %v\nwant %v", got, want)
	}
}

func TestPipeline_RunEdges(t *testing.T) {
	if got, err := Run(context.Background(), nil, 4, double); got != nil || err != nil {
		t.Errorf("Run(nil) = (%v, %v), want (nil, nil)", got, err)
	}

	// workers <= 0 当作 1，不能 panic 也不能死锁
	for _, w := range []int{0, -3} {
		got, err := Run(context.Background(), []int{1, 2, 3}, w, double)
		if err != nil {
			t.Fatalf("workers=%d 报错: %v", w, err)
		}
		if !slices.Equal(got, []int{2, 4, 6}) {
			t.Errorf("workers=%d 结果 = %v, want [2 4 6]", w, got)
		}
	}
}

// TestPipeline_RunRespectsLimit 验证 fan-out 的并发上限真的生效。
func TestPipeline_RunRespectsLimit(t *testing.T) {
	for _, workers := range []int{1, 2, 5} {
		var maxSeen atomic.Int64
		items := make([]int, 20)
		for i := range items {
			items[i] = i
		}

		if _, err := Run(context.Background(), items, workers, slowFunc(10*time.Millisecond, &maxSeen)); err != nil {
			t.Fatalf("Run 失败: %v", err)
		}
		if got := int(maxSeen.Load()); got > workers {
			t.Errorf("workers=%d 时最大并发 = %d，超限了", workers, got)
		}
		if got := int(maxSeen.Load()); workers > 1 && got < 2 {
			t.Errorf("workers=%d 时最大并发只有 %d —— 是不是根本没并发起来？", workers, got)
		}
	}
}

// TestPipeline_RunCanceled 验证主动取消。
func TestPipeline_RunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	items := make([]int, 100)

	go func() { time.Sleep(20 * time.Millisecond); cancel() }()

	var maxSeen atomic.Int64
	start := time.Now()
	got, err := Run(ctx, items, 2, slowFunc(5*time.Millisecond, &maxSeen))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("取消之后 Run 应该返回错误")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false，err = %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Error("这是主动取消，不是超时")
	}
	if got != nil {
		t.Errorf("取消时应该返回 nil 结果，实际 %d 个", len(got))
	}
	// 100 个 × 5ms ÷ 2 worker = 250ms 才能跑完；取消后应该远早于此
	if elapsed > 150*time.Millisecond {
		t.Errorf("取消后 %v 才返回 —— 应该立刻停，而不是跑完所有元素", elapsed)
	}
}

// TestPipeline_RunTimeout 验证超时，且能和主动取消区分开。
func TestPipeline_RunTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	items := make([]int, 100)
	var maxSeen atomic.Int64
	_, err := Run(ctx, items, 2, slowFunc(5*time.Millisecond, &maxSeen))

	if err == nil {
		t.Fatal("超时之后 Run 应该返回错误")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false，err = %v\n"+
			"（直接返回 ctx.Err() 就能区分两者）", err)
	}
}

// TestPipeline_RunCancelNoLeak 是今天的题眼：取消之后一条 goroutine 都不能剩。
func TestPipeline_RunCancelNoLeak(t *testing.T) {
	assertNoLeak(t, func() {
		for range 20 {
			ctx, cancel := context.WithCancel(context.Background())
			items := make([]int, 200)
			go func() { time.Sleep(5 * time.Millisecond); cancel() }()
			_, _ = Run(ctx, items, 4, double)
			cancel()
		}
	})
}

// TestPipeline_RunAlreadyCanceled 验证「传进来时就已经取消了」。
func TestPipeline_RunAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消

	assertNoLeak(t, func() {
		for range 20 {
			_, err := Run(ctx, []int{1, 2, 3, 4, 5}, 3, double)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("已取消的 ctx 应该立刻返回 Canceled，实际 %v", err)
			}
		}
	})
}
