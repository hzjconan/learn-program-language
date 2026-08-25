// Command chandemo 是 lessons/D8.md 的可运行验证。
//
// 核心要建立两个直觉：
//
//  1. **无缓冲 channel 传的不只是值，还是「时序」** —— 收发双方必须碰头
//
//  2. **goroutine 泄漏没有任何症状** —— 不 panic、不报错，只是悄悄不退出
//
//     go run ./cmd/chandemo
package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

// ---------- ① main 不等任何人 ----------

func demoMainDoesNotWait() {
	title("① main 返回时，goroutine 直接被丢弃")

	for i := range 3 {
		go fmt.Println("  我是 goroutine", i, "—— 你多半看不到这行")
	}
	fmt.Println("  主流程干完了。没有 sleep / WaitGroup 的话，上面三行大概率不会打印。")
	time.Sleep(10 * time.Millisecond) // 只为了演示，真实代码别用 sleep 同步
	fmt.Println("  （这里 sleep 了 10ms，所以你可能看到了它们）")
}

// ---------- ② 无缓冲 = 同步会合 ----------

func demoUnbuffered() {
	title("② 无缓冲 channel：收发双方必须【同时】就绪")

	ch := make(chan string)

	go func() {
		fmt.Println("  [发送方] 我准备好了，开始发送……（会卡在这儿）")
		ch <- "货物"
		fmt.Println("  [发送方] 发送【返回了】—— 说明接收方确实拿到了")
	}()

	time.Sleep(50 * time.Millisecond)
	fmt.Println("  [接收方] 我睡了 50ms 才来取")
	fmt.Println("  [接收方] 取到:", <-ch)
	time.Sleep(10 * time.Millisecond)

	fmt.Println()
	fmt.Println("  ⭐ 发送方那句「发送返回了」是在接收之后才打印的。")
	fmt.Println("     无缓冲 channel 传的不只是值，还是【时序保证】：")
	fmt.Println("     发送方一旦返回，就确知对方收到了。Java 的 BlockingQueue 给不了这个。")
}

// ---------- ③ 有缓冲 = 队列 ----------

func demoBuffered() {
	title("③ 有缓冲 channel：满了才阻塞")

	ch := make(chan int, 2)
	fmt.Printf("  make(chan int, 2)  len=%d cap=%d\n", len(ch), cap(ch))

	ch <- 1
	ch <- 2
	fmt.Printf("  连发两个都没阻塞  len=%d cap=%d\n", len(ch), cap(ch))

	select {
	case ch <- 3:
		fmt.Println("  第三个也放进去了？不应该")
	default:
		fmt.Println("  第三个放不进去了 —— 缓冲区满，发送会阻塞（这里用 default 探测）")
	}

	fmt.Println("  取出:", <-ch, <-ch)
	fmt.Printf("  取空之后      len=%d cap=%d\n", len(ch), cap(ch))
}

// ---------- ④ 关闭语义 ----------

func demoClose() {
	title("④ 关闭 = 「不会再有新值了」，不是「立刻停止」")

	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	close(ch)

	v1, ok1 := <-ch
	v2, ok2 := <-ch
	v3, ok3 := <-ch
	fmt.Printf("  close 之后依次接收: (%d,%v) (%d,%v) (%d,%v)\n", v1, ok1, v2, ok2, v3, ok3)
	fmt.Println("  ⭐ 缓冲区里的货照样取得完；取空之后才 ok=false，返回零值")

	fmt.Println()
	fmt.Println("  三条会 panic 的操作：")
	fmt.Println("    close(已关闭的 ch)  → panic: close of closed channel")
	fmt.Println("    已关闭的 ch <- v    → panic: send on closed channel")
	fmt.Println("    接收不会 panic      ← 只有接收是安全的")
	fmt.Println("  所以规则是：【只有发送方能关闭】。")

	// for range 读到关闭为止
	ch2 := make(chan int)
	go func() {
		for i := range 3 {
			ch2 <- i
		}
		close(ch2) // ⭐ 不关的话下面的 for range 永远不退出
	}()
	fmt.Print("  for range 读取: ")
	for v := range ch2 {
		fmt.Print(v, " ")
	}
	fmt.Println("← close 之后循环自动退出")
}

// ---------- ⑤ select ----------

func demoSelect() {
	title("⑤ select：多个就绪时【随机】选，不是从上到下")

	a := make(chan int, 100)
	b := make(chan int, 100)
	for range 100 {
		a <- 1
		b <- 2
	}

	countA, countB := 0, 0
	for range 100 {
		select {
		case <-a:
			countA++
		case <-b:
			countB++
		}
	}
	fmt.Printf("  两个 channel 都一直就绪，100 次 select 的结果: a=%d b=%d\n", countA, countB)
	fmt.Println("  ⭐ 接近五五开 —— 这是刻意随机化的，防止某个 case 被饿死")

	// default
	empty := make(chan int)
	select {
	case <-empty:
		fmt.Println("  不可能走到这儿")
	default:
		fmt.Println("  有 default → 非阻塞，没人就绪就立刻走 default")
	}

	// 超时
	slow := make(chan int)
	start := time.Now()
	select {
	case <-slow:
		fmt.Println("  不可能")
	case <-time.After(30 * time.Millisecond):
		fmt.Printf("  超时模式：等了 %v 就放弃了\n", time.Since(start).Round(10*time.Millisecond))
	}
}

// ---------- ⑥ nil channel ----------

func demoNilChannel() {
	title("⑥ nil channel：读写都永远阻塞（这是个特性，不是 bug）")

	var nilCh chan int
	select {
	case <-nilCh:
		fmt.Println("  不可能")
	default:
		fmt.Println("  从 nil channel 接收 → 永远阻塞，所以走了 default")
	}

	fmt.Println()
	fmt.Println("  用途：在 select 里把某个 channel 置 nil，等于【动态禁用这个 case】。")
	fmt.Println("  合并两个 channel 时，某个关闭了就把它置 nil，")
	fmt.Println("  否则从已关闭的 channel 读会立刻返回零值，select 会疯狂空转。")
}

// ---------- ⑦ goroutine 泄漏 —— 今天的题眼 ----------

// leakyWork 是【错误写法】：无缓冲 channel + 调用方可能不读。
func leakyWork() <-chan int {
	ch := make(chan int) // ⚠️ 无缓冲
	go func() {
		time.Sleep(20 * time.Millisecond)
		ch <- 42 // 没人读的话，这条 goroutine 永远卡在这里
	}()
	return ch
}

// safeWork 是【正确写法】：缓冲区 1，发送方总能放下就走。
func safeWork() <-chan int {
	ch := make(chan int, 1) // ⭐ 容量 1
	go func() {
		time.Sleep(20 * time.Millisecond)
		ch <- 42 // 无论有没有人读，都能立刻返回
	}()
	return ch
}

func demoLeak() {
	title("⑦ goroutine 泄漏：不 panic、不报错，只是悄悄不退出")

	base := runtime.NumGoroutine()
	fmt.Printf("  起点 goroutine 数量: %d\n\n", base)

	// 模拟「等 5ms 就超时放弃」的调用方
	for range 50 {
		select {
		case <-leakyWork():
		case <-time.After(5 * time.Millisecond): // 超时，不再读那个 channel
		}
	}
	time.Sleep(50 * time.Millisecond) // 给它们充分的时间「完成」
	fmt.Printf("  调用 50 次【无缓冲】版并超时放弃后: %d  ← 泄漏了\n", runtime.NumGoroutine())

	for range 50 {
		select {
		case <-safeWork():
		case <-time.After(5 * time.Millisecond):
		}
	}
	time.Sleep(50 * time.Millisecond)
	fmt.Printf("  再调用 50 次【缓冲 1】版并超时放弃后: %d  ← 没有增长\n", runtime.NumGoroutine())

	fmt.Println()
	fmt.Println("  ⭐ 两段代码跑起来【一模一样】：都不报错，结果都对。")
	fmt.Println("     差别只有 make(chan int) vs make(chan int, 1)。")
	fmt.Println("     线上的表现是内存缓慢上涨、goroutine 数单调增长，跑几天才炸。")
}

// ---------- ⑧ worker pool ----------

func demoWorkerPool() {
	title("⑧ worker pool：今天主练习的骨架")

	jobs := make(chan int)
	results := make(chan string)

	var wg sync.WaitGroup
	for w := 1; w <= 3; w++ {
		wg.Go(func() {
			for j := range jobs { // ← jobs 关闭后自动退出
				time.Sleep(10 * time.Millisecond)
				results <- fmt.Sprintf("worker%d 处理了 job%d", w, j)
			}
		})
	}

	// ⭐ 必须单独起一条 goroutine 等所有 worker 结束
	go func() {
		wg.Wait()
		close(results) // ← 让下面的 for range 能退出
	}()

	// 投递
	go func() {
		for j := 1; j <= 6; j++ {
			jobs <- j
		}
		close(jobs) // ← 告诉 worker 没活了
	}()

	n := 0
	for r := range results {
		n++
		fmt.Println("  " + r)
	}
	fmt.Printf("  共收到 %d 个结果\n", n)

	fmt.Println()
	fmt.Println("  三个关键点：")
	fmt.Println("    1. close(jobs)   → worker 的 for range 自然退出")
	fmt.Println("    2. wg.Wait() 必须在【单独的 goroutine】里 —— 否则主流程还没开始读")
	fmt.Println("       results 就先阻塞了，直接死锁")
	fmt.Println("    3. close(results) → 主流程的 for range 退出")
}

func main() {
	demoMainDoesNotWait()
	demoUnbuffered()
	demoBuffered()
	demoClose()
	demoSelect()
	demoNilChannel()
	demoLeak()
	demoWorkerPool()
}
