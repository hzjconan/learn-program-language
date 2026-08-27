package racefix

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// 测试文件是我写的，别改。你的修复要让它们在 -race 下全绿。
//
// 你自己想补的用例写在 stats_extra_test.go 里，函数名用 TestRacefix_Extra_Xxx。

// ---------- Counter ----------

func TestRacefix_CounterExact(t *testing.T) {
	const goroutines, each = 50, 200
	var c Counter

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range each {
				c.Inc()
			}
		})
	}
	wg.Wait()

	if want := int64(goroutines * each); c.Value() != want {
		t.Errorf("Value() = %d, want %d（丢了 %d 次自增）\n"+
			"n++ 是【读-改-写】三步，不是原子操作", c.Value(), want, want-c.Value())
	}
}

// TestRacefix_CounterZeroValueUsable 确认修复之后零值仍然可用（D4 §1）。
func TestRacefix_CounterZeroValueUsable(t *testing.T) {
	var c Counter // 没有 NewCounter()
	c.Inc()
	if c.Value() != 1 {
		t.Errorf("零值 Counter 用不了：Value() = %d, want 1", c.Value())
	}
}

// ---------- Registry ----------

func TestRacefix_RegistryExact(t *testing.T) {
	const goroutines, each = 20, 100
	r := NewRegistry()

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			name := fmt.Sprintf("svc%d", g%4) // 4 个服务名，制造真实竞争
			for range each {
				r.Add(name)
			}
		})
	}
	wg.Wait()

	for i := range 4 {
		name := fmt.Sprintf("svc%d", i)
		want := (goroutines / 4) * each
		if got := r.Count(name); got != want {
			t.Errorf("Count(%q) = %d, want %d", name, got, want)
		}
	}
}

// TestRacefix_SnapshotIsolated 验证 Snapshot 返回的 map 不能影响内部状态。
//
// -race 不一定报得出来（要正好并发访问才行），但这个断言一定抓得住。
func TestRacefix_SnapshotIsolated(t *testing.T) {
	r := NewRegistry()
	r.Add("a")
	r.Add("a")

	snap := r.Snapshot()
	snap["a"] = 999
	snap["注入的键"] = 1

	if got := r.Count("a"); got != 2 {
		t.Errorf("调用方改了 Snapshot 的返回值，污染了内部数据：Count(\"a\") = %d, want 2", got)
	}
	if got := r.Snapshot()["注入的键"]; got != 0 {
		t.Error("调用方往 Snapshot 的返回值里加了键，污染了内部数据")
	}
}

// TestRacefix_SnapshotWhileWriting 在并发写的同时反复取快照。
//
// 如果 Snapshot 返回的是内部 map 本身，调用方遍历它的时候另一条 goroutine 正在写 ——
// 轻则 -race 报警，重则 fatal error: concurrent map iteration and map write。
func TestRacefix_SnapshotWhileWriting(t *testing.T) {
	r := NewRegistry()
	done := make(chan struct{})

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				r.Add("hot")
			}
		}
	})

	for range 200 {
		for range r.Snapshot() { // 遍历快照
			_ = 0
		}
	}
	close(done)
	wg.Wait()
}

// ---------- LoadConfig ----------

func TestRacefix_LoadConfigOnce(t *testing.T) {
	const goroutines = 100

	var wg sync.WaitGroup
	got := make([]*Config, goroutines)
	for i := range goroutines {
		wg.Go(func() { got[i] = LoadConfig() })
	}
	wg.Wait()

	if LoadHit != 1 {
		t.Errorf("初始化执行了 %d 次, want 1", LoadHit)
	}
	for i, c := range got {
		if c == nil {
			t.Fatalf("第 %d 条 goroutine 拿到了 nil", i)
		}
		// ⭐ 这一条抓的是「拿到了半初始化的对象」
		if c.Name != "prod" || c.Retries != 3 {
			t.Fatalf("第 %d 条 goroutine 拿到了不完整的配置: %+v\n"+
				"（手写双检锁的经典后果：指针已赋值，但对象字段还没初始化完）", i, *c)
		}
	}
}

// ---------- Aggregate ----------

func TestRacefix_AggregateExact(t *testing.T) {
	samples := make([]Sample, 60)
	wantTotal := int64(0)
	for i := range samples {
		samples[i] = Sample{Service: fmt.Sprintf("svc%d", i%3), Millis: i}
		wantTotal += int64(i)
	}

	total, reg := Aggregate(samples)

	if total != wantTotal {
		t.Errorf("total = %d, want %d", total, wantTotal)
	}
	for i := range 3 {
		name := fmt.Sprintf("svc%d", i)
		if got := reg.Count(name); got != 20 {
			t.Errorf("Count(%q) = %d, want 20", name, got)
		}
	}
}

// TestRacefix_AggregateWaitsForAll 抓的是 wg.Add 放错位置。
//
// wg.Add 写在 goroutine 内部时，Wait 可能在任何一条 goroutine 执行到 Add 之前就返回 ——
// 计数器还是 0，Wait 直接放行。这种 bug -race 报不出来，只能靠数值断言。
func TestRacefix_AggregateWaitsForAll(t *testing.T) {
	samples := make([]Sample, 40)
	for i := range samples {
		samples[i] = Sample{Service: "svc", Millis: 1}
	}

	// 跑多次，提高「Wait 提前返回」的暴露概率
	for attempt := range 20 {
		total, reg := Aggregate(samples)
		if total != 40 || reg.Count("svc") != 40 {
			t.Fatalf("第 %d 次: total=%d count=%d, want 40 和 40\n"+
				"（Wait 在所有 goroutine 完成前就返回了 —— wg.Add 的位置对吗？）",
				attempt, total, reg.Count("svc"))
		}
	}
}

// TestRacefix_AggregateIsConcurrent 验证修复之后仍然是并发的。
//
// ⚠️ 别为了消除竞态就把整个函数串行化 —— 那样测试会因为太慢而失败。
func TestRacefix_AggregateIsConcurrent(t *testing.T) {
	samples := make([]Sample, 50)
	for i := range samples {
		samples[i] = Sample{Service: "svc", Millis: 1}
	}

	start := time.Now()
	Aggregate(samples)
	elapsed := time.Since(start)

	// 每个 sample sleep 1ms，串行要 50ms 以上；并发应该远小于这个数
	if elapsed > 25*time.Millisecond {
		t.Errorf("Aggregate 花了 %v —— 50 个 sample 各 sleep 1ms，"+
			"并发跑不该超过 25ms。是不是加了一把锁把整个函数串行化了？", elapsed)
	}
}
