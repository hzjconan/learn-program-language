// Command syncdemo 是 lessons/D9.md 的可运行验证。
//
// 核心要建立的直觉：**锁不只是「防止同时进入」，它还负责「把我的写入让对方看见」。**
// 后半句是 happens-before，也是最容易被忽略的一半。
//
//	go run ./cmd/syncdemo
//	go run -race ./cmd/syncdemo    # ⭐ 加 -race 再跑一遍，输出完全不同
package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

// ---------- ① 竞态的后果：丢更新 ----------

func demoLostUpdate() {
	title("① 无保护的 n++ 会丢更新")

	const goroutines, each = 50, 1000
	want := goroutines * each

	var unsafe int
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range each {
				unsafe++ //nolint:staticcheck // 故意演示竞态
			}
		})
	}
	wg.Wait()

	var safe atomic.Int64
	var wg2 sync.WaitGroup
	for range goroutines {
		wg2.Go(func() {
			for range each {
				safe.Add(1)
			}
		})
	}
	wg2.Wait()

	fmt.Printf("  期望值           = %d\n", want)
	fmt.Printf("  无保护的 n++     = %d   ← 丢了 %d 次\n", unsafe, want-unsafe)
	fmt.Printf("  atomic.Int64.Add = %d   ✅\n", safe.Load())
	fmt.Println()
	fmt.Println("  n++ 不是一条指令，是【读-改-写】三步。两条 goroutine 同时读到 5，")
	fmt.Println("  各自算出 6，各自写回 6 —— 两次自增只生效了一次。")
}

// ---------- ② Mutex 不可重入 ----------

type reentrantTrap struct {
	mu sync.Mutex
	n  int
}

// Inc 是导出方法，负责加锁。
func (r *reentrantTrap) Inc() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.incLocked() // ✅ 调不加锁的内部版本
}

// incLocked 假定调用方已经持有锁 —— 名字里的 Locked 就是这个约定。
func (r *reentrantTrap) incLocked() { r.n++ }

func demoNonReentrant() {
	title("② Go 的锁【不可重入】")

	r := &reentrantTrap{}
	r.Inc()
	fmt.Printf("  正确写法：Inc() 加锁 → 调 incLocked()（不加锁）→ n = %d\n", r.n)
	fmt.Println()
	fmt.Println("  ⚠️ 如果 Inc() 里调另一个也 Lock() 的导出方法，会【自锁死】：")
	fmt.Println("       fatal error: all goroutines are asleep - deadlock!")
	fmt.Println("       goroutine 1 [sync.Mutex.Lock]")
	fmt.Println()
	fmt.Println("  Java 的 synchronized 是可重入的，Go 不是。")
	fmt.Println("  惯例：【导出方法加锁，内部方法不加锁】，不加锁的版本命名带 Locked 后缀。")
}

// ---------- ③ 拷贝锁 ----------

func demoCopyLock() {
	title("③ 含 Mutex 的 struct 不能拷贝")

	fmt.Println("  下面这个写法 go vet 会直接拦下：")
	fmt.Println()
	fmt.Println("      func (c Cache) Bad() { ... }      // 值接收者")
	fmt.Println("      → passes lock by value: Cache contains sync.Mutex (copylocks)")
	fmt.Println()
	fmt.Println("  拷贝会把【锁的状态】一起复制，两份锁互不知情：")
	fmt.Println("    goroutine A 锁住原件，goroutine B 锁住副本 —— 两边都以为自己独占了。")
	fmt.Println()
	fmt.Println("  ⭐ 所以含 sync.Mutex 的类型，方法必须【全用指针接收者】（D4 §3）。")
}

// ---------- ④ RWMutex vs Mutex ----------

type withMutex struct {
	mu sync.Mutex
	m  map[int]int
}

func (s *withMutex) get(k int) int { s.mu.Lock(); defer s.mu.Unlock(); return s.m[k] }

type withRW struct {
	mu sync.RWMutex
	m  map[int]int
}

func (s *withRW) get(k int) int { s.mu.RLock(); defer s.mu.RUnlock(); return s.m[k] }

func demoRWMutex() {
	title("④ RWMutex 什么时候划算：看【并发读者数量】")

	m := map[int]int{1: 1}
	a := &withMutex{m: m}
	b := &withRW{m: m}

	bench := func(get func(int) int, readers, each int) time.Duration {
		start := time.Now()
		var wg sync.WaitGroup
		for range readers {
			wg.Go(func() {
				for range each {
					_ = get(1)
				}
			})
		}
		wg.Wait()
		return time.Since(start).Round(time.Millisecond)
	}

	const total = 400000
	fmt.Printf("  %-10s %10s %10s %s\n", "并发读者数", "Mutex", "RWMutex", "谁快")
	for _, readers := range []int{1, 2, 8} {
		each := total / readers
		tm := bench(a.get, readers, each)
		tr := bench(b.get, readers, each)
		winner := "Mutex 快"
		if tr < tm {
			winner = "RWMutex 快"
		}
		fmt.Printf("  %-10d %10v %10v %s\n", readers, tm, tr, winner)
	}

	fmt.Println()
	fmt.Println("  ⭐ 规律：1~2 个读者时两者【基本持平】（差异在噪声范围内），")
	fmt.Println("     8 个读者时 RWMutex 才明显胜出 —— 因为多个读者能【真正并行】，")
	fmt.Println("     而 Mutex 把它们排成一队。优势随并发读者数增长，不是天生就快。")
	fmt.Println("     （单次运行的毫秒数噪声不小，看趋势别看绝对值 —— D6 §6 那条）")
	fmt.Println()
	fmt.Println("  但别默认用 RWMutex，三个前提要同时满足：")
	fmt.Println("    1. 读远多于写（有写者时读者照样要等）")
	fmt.Println("    2. 并发读者数量确实多")
	fmt.Println("    3. 临界区不是极短（否则维护读者计数的开销就占大头了）")
	fmt.Println("  先用 Mutex，被 benchmark 证明是瓶颈了再换。")
}

// ---------- ⑤ sync.Once ----------

var (
	// initCount 是【普通 int】，不是 atomic —— 因为它的所有读写之间都有 happens-before 链：
	//
	//   initCount++（在 once.Do 内，只执行一次）
	//     ↓ once.Do 返回 happens-before 任何其他 Do 返回
	//   loadConfig 返回 → wg.Done()
	//     ↓ wg.Wait() 返回
	//   读 initCount                                    ✅ 安全
	//
	// ⚠️ 少了 wg.Wait() 那一环链就断了 —— 那时 -race 会立刻报警。
	// once.Do 只保证「函数执行一次」和「写入对后续 Do 的调用者可见」，
	// 它管不了 Do 外面任意位置的读。
	initCount int
	once      sync.Once
	config    string
)

func loadConfig() string {
	once.Do(func() {
		initCount++
		time.Sleep(5 * time.Millisecond) // 模拟初始化耗时
		config = "已加载"
	})
	return config
}

func demoOnce() {
	title("⑤ sync.Once：只执行一次，且别人会等它执行完")

	var wg sync.WaitGroup
	results := make([]string, 20)
	for i := range 20 {
		wg.Go(func() { results[i] = loadConfig() })
	}
	wg.Wait()

	allLoaded := true
	for _, r := range results {
		if r != "已加载" {
			allLoaded = false
		}
	}
	fmt.Printf("  20 条 goroutine 并发调用，初始化执行了 %d 次\n", initCount)
	fmt.Printf("  所有 goroutine 都拿到了完整的配置: %v\n", allLoaded)
	fmt.Println()
	fmt.Println("  ⭐ 关键不只是「只执行一次」，还有【其他人会阻塞等它执行完】——")
	fmt.Println("     不会拿到半初始化的对象。手写双检锁做不到这一点（见 §5）。")
	fmt.Println()
	fmt.Println("  ⭐ 注意 initCount 是【普通 int】不是 atomic —— 因为它的所有读写之间")
	fmt.Println("     都有完整的 happens-before 链（once.Do → wg.Done → wg.Wait → 读）。")
	fmt.Println("     判断要不要加保护，看的是【所有读和写】，不是只看写的那一侧。")
	fmt.Println("     Go 1.21+ 还有更简洁的 sync.OnceValue。")
}

// ---------- ⑥ atomic vs mutex ----------

func demoAtomicVsMutex() {
	title("⑥ 单个计数器：atomic 比 mutex 快")

	const goroutines, each = 8, 200000

	var mu sync.Mutex
	var n1 int64
	start := time.Now()
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range each {
				mu.Lock()
				n1++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	mutexTime := time.Since(start)

	var n2 atomic.Int64
	start = time.Now()
	var wg2 sync.WaitGroup
	for range goroutines {
		wg2.Go(func() {
			for range each {
				n2.Add(1)
			}
		})
	}
	wg2.Wait()
	atomicTime := time.Since(start)

	fmt.Printf("  Mutex  : %v  (n=%d)\n", mutexTime.Round(time.Millisecond), n1)
	fmt.Printf("  atomic : %v  (n=%d)\n", atomicTime.Round(time.Millisecond), n2.Load())
	fmt.Println()
	fmt.Println("  ⚠️ 但 atomic 只保护【单个变量】。要保证多个变量之间的一致性，必须用锁：")
	fmt.Println("       total.Add(1); sum.Add(v)   ← 两行之间别人可能读到不匹配的状态")
}

// ---------- ⑦ happens-before —— 今天的题眼 ----------

func demoHappensBefore() {
	title("⑦ 锁的另一半职责：让对方【看见】你的写入")

	fmt.Println("  经典的错误代码：")
	fmt.Println()
	fmt.Println("      var done bool")
	fmt.Println("      var msg string")
	fmt.Println("      go func() { msg = \"hello\"; done = true }()")
	fmt.Println("      for !done { }              // 忙等")
	fmt.Println("      fmt.Println(msg)")
	fmt.Println()
	fmt.Println("  两个问题，都和「互斥」无关：")
	fmt.Println("    1. 循环可能【永远不退出】—— 编译器可以把 done 缓存进寄存器，")
	fmt.Println("       把 for !done {} 优化成 if !done { for {} }")
	fmt.Println("    2. 就算退出了，msg 也可能是【空字符串】—— 两次写入的可见顺序没有保证")
	fmt.Println()
	fmt.Println("  ⭐ 加一个 channel 或 mutex 就都解决了，")
	fmt.Println("     不是因为「互斥」，而是因为建立了 happens-before：")
	fmt.Println("       mu.Unlock() happens-before 下一次 mu.Lock() 返回")
	fmt.Println("       channel 发送 happens-before 对应的接收完成")
	fmt.Println()
	fmt.Println("  很多人以为「这个变量只有一处写、一处读，不会冲突，不用加锁」——")
	fmt.Println("  但【没有锁就没有 happens-before，读那一侧可能永远看不到那次写入】。")

	// 正确版本：用 channel 建立 happens-before
	done := make(chan struct{})
	var msg string
	go func() {
		msg = "hello"
		close(done) // close happens-before 收到零值
	}()
	<-done
	fmt.Printf("\n  用 channel 同步之后: msg = %q ✅\n", msg)
}

// ---------- ⑧ errgroup ----------

func demoErrgroup() {
	title("⑧ errgroup：WaitGroup + 错误传播 + 并发上限")

	var g errgroup.Group
	g.SetLimit(3) // 并发上限，等价于 D8 手写的 worker pool

	var inFlight, maxSeen atomic.Int64
	for i := range 9 {
		g.Go(func() error {
			n := inFlight.Add(1)
			for {
				old := maxSeen.Load()
				if n <= old || maxSeen.CompareAndSwap(old, n) {
					break
				}
			}
			defer inFlight.Add(-1)

			time.Sleep(10 * time.Millisecond)
			if i == 4 {
				return errors.New("任务 4 失败了")
			}
			return nil
		})
	}

	err := g.Wait()
	fmt.Printf("  9 个任务，SetLimit(3) → 同时在跑的最大数量 = %d\n", maxSeen.Load())
	fmt.Printf("  g.Wait() 返回的错误 = %v\n", err)
	fmt.Println()
	fmt.Println("  比手写 WaitGroup 强在三点：")
	fmt.Println("    1. 任务返回 error，Wait 把【第一个】非 nil 错误交给你")
	fmt.Println("    2. errgroup.WithContext 会在第一个错误时自动取消 ctx（D10）")
	fmt.Println("    3. SetLimit 就是并发上限，不用自己写 worker pool")
	fmt.Println()
	fmt.Println("  ⚠️ 注意 Wait 仍然会等【所有】任务结束，不是一出错就立刻返回。")
	fmt.Println("     「让其他任务提前退出」靠的是 ctx 取消，不是 errgroup 本身。")
}

func main() {
	demoLostUpdate()
	demoNonReentrant()
	demoCopyLock()
	demoRWMutex()
	demoOnce()
	demoAtomicVsMutex()
	demoHappensBefore()
	demoErrgroup()

	fmt.Println()
	fmt.Println("──────────────────────────────────────────")
	fmt.Println("⭐ 现在用 go run -race ./cmd/syncdemo 再跑一遍，看第 ① 段的报告。")
}
