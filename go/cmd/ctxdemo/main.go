// Command ctxdemo 是 lessons/D10.md 的可运行验证。
//
// 核心要建立的直觉：**cancel() 不会停掉任何东西，它只是关了一个 channel。**
// 所有的「响应取消」都是接收方自己写的代码。
//
//	go run ./cmd/ctxdemo
package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

// ---------- ① Done() 就是个 channel ----------

func demoDoneIsChannel() {
	title("① 取消的全部机制：关闭一个 channel")

	ctx, cancel := context.WithCancel(context.Background())

	fmt.Printf("  cancel 之前: ctx.Err() = %v\n", ctx.Err())
	select {
	case <-ctx.Done():
		fmt.Println("  Done() 已就绪？不应该")
	default:
		fmt.Println("  Done() 还没就绪（channel 没关）")
	}

	cancel()

	fmt.Printf("  cancel 之后: ctx.Err() = %v\n", ctx.Err())
	select {
	case <-ctx.Done():
		fmt.Println("  Done() 就绪了 ← channel 被关闭了")
	default:
		fmt.Println("  ？")
	}

	cancel() // 再调一次
	fmt.Println("  ⭐ cancel 可以重复调用，是幂等的（内部有 sync.Once 类似的保护）")
}

// ---------- ② 取消沿树传播 ----------

func demoPropagation() {
	title("② 父取消 → 所有后代一起取消；子取消不影响父")

	root, cancelRoot := context.WithCancel(context.Background())
	child1, cancel1 := context.WithTimeout(root, time.Hour) // 自己的超时是 1 小时
	child2, cancel2 := context.WithCancel(root)
	grand, cancelG := context.WithCancel(child1)
	defer func() { cancel1(); cancel2(); cancelG() }()

	cancelRoot() // 只取消根
	time.Sleep(5 * time.Millisecond)

	report := func(name string, c context.Context) {
		fmt.Printf("  %-8s Err() = %v\n", name, c.Err())
	}
	report("root", root)
	report("child1", child1)
	report("child2", child2)
	report("grand", grand)
	fmt.Println("  ⭐ child1 自己的超时是 1 小时，但父一取消它立刻就完了")

	// 反过来
	p, cancelP := context.WithCancel(context.Background())
	c, cancelC := context.WithCancel(p)
	defer cancelP()
	cancelC()
	fmt.Printf("\n  只取消子: 子 Err()=%v, 父 Err()=%v  ← 父不受影响\n", c.Err(), p.Err())
}

// ---------- ③ 超时取更早的那个 ----------

func demoDeadline() {
	title("③ 子的超时不能超过父：取更早的那个")

	parent, cancelP := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelP()
	child, cancelC := context.WithTimeout(parent, 10*time.Second) // 想要 10 秒
	defer cancelC()

	if dl, ok := child.Deadline(); ok {
		fmt.Printf("  子 context 申请了 10 秒，实际剩余: %v\n", time.Until(dl).Round(10*time.Millisecond))
	}

	start := time.Now()
	<-child.Done()
	fmt.Printf("  子实际在 %v 后就绪，Err() = %v\n",
		time.Since(start).Round(10*time.Millisecond), child.Err())
	fmt.Println("  ⭐ 在请求入口设一个总超时，下游怎么设都突破不了它")
}

// ---------- ④ 取消 vs 超时 ----------

func demoErrKinds() {
	title("④ 区分「主动取消」和「超时」")

	c1, cancel1 := context.WithCancel(context.Background())
	cancel1()

	c2, cancel2 := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel2()
	<-c2.Done()

	for name, c := range map[string]context.Context{"主动取消": c1, "超时": c2} {
		fmt.Printf("  %-8s Err()=%-25v  Is(Canceled)=%-5v  Is(DeadlineExceeded)=%v\n",
			name, c.Err(),
			errors.Is(c.Err(), context.Canceled),
			errors.Is(c.Err(), context.DeadlineExceeded))
	}
	fmt.Println("  ⭐ 用 errors.Is 判断，别用 == （返回值可能被包装过）")
}

// ---------- ⑤ 取消不会停掉任何东西 —— 今天的题眼 ----------

func demoCooperative() {
	title("⑤ cancel() 停不掉任何东西 —— 全靠接收方自己配合")

	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())

	var 听话 atomic.Bool
	var 不听话 atomic.Bool

	go func() { // 检查 ctx
		for {
			select {
			case <-ctx.Done():
				听话.Store(true)
				return
			default:
				time.Sleep(time.Millisecond)
			}
		}
	}()

	go func() { // 完全不检查 ctx
		time.Sleep(300 * time.Millisecond)
		不听话.Store(true)
	}()

	time.Sleep(20 * time.Millisecond)
	fmt.Printf("  cancel 之前 goroutine 数: %d\n", runtime.NumGoroutine())
	cancel()
	time.Sleep(50 * time.Millisecond)

	fmt.Printf("  cancel 之后 50ms:  听话的退出了=%v  不听话的退出了=%v  goroutine 数=%d\n",
		听话.Load(), 不听话.Load(), runtime.NumGoroutine())
	time.Sleep(300 * time.Millisecond)
	fmt.Printf("  再等 300ms:        不听话的自己跑完了=%v  goroutine 数=%d（基线 %d）\n",
		不听话.Load(), runtime.NumGoroutine(), base)

	fmt.Println()
	fmt.Println("  ⭐ cancel() 只做一件事：关闭 ctx.Done() 那个 channel。")
	fmt.Println("     它不停 goroutine、不中断函数、不关连接、不回滚事务。")
	fmt.Println("     一个不检查 Done() 的循环，cancel 一万次也拦不住。")
}

// ---------- ⑥ WithValue 的正确姿势 ----------

type traceIDKey struct{} // ⭐ 自定义类型，不可能和别的包冲突

// WithTraceID 把 trace ID 放进 ctx。
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceIDFrom 取出 trace ID，类型安全。
func TraceIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(traceIDKey{}).(string)
	return id, ok
}

func demoValue() {
	title("⑥ WithValue：只放请求域数据，且要包一层类型安全的函数")

	ctx := WithTraceID(context.Background(), "trace-abc-123")
	if id, ok := TraceIDFrom(ctx); ok {
		fmt.Printf("  取出 trace ID: %s\n", id)
	}
	if _, ok := TraceIDFrom(context.Background()); !ok {
		fmt.Println("  空 ctx 里取不到，ok=false（不会 panic）")
	}

	fmt.Println()
	fmt.Println("  ⚠️ 别拿它当依赖注入容器：")
	fmt.Println("    · 没有类型安全 —— Value 返回 any，拿错了是运行时 panic 或静默零值")
	fmt.Println("    · 依赖变隐式 —— 签名 f(ctx) 看不出它需要什么，只能读实现")
	fmt.Println("    · 删掉一个依赖编译照过，运行时才炸")
	fmt.Println("  ⭐ 依赖用构造函数注入（D14 会正面讲）。")
}

// ---------- ⑦ 忘了 cancel 会怎样 ----------

func demoLostCancel() {
	title("⑦ 忘了 defer cancel() 的后果")

	fmt.Println("  ctx, cancel := context.WithTimeout(parent, time.Hour)")
	fmt.Println("  // 忘了 defer cancel()")
	fmt.Println()
	fmt.Println("  后果：即使函数早就返回了，parent 仍然【持有这个子节点】，")
	fmt.Println("        直到那 1 小时到期才释放 —— 高频调用就是内存泄漏。")
	fmt.Println()
	fmt.Println("  好消息：go vet 的 lostcancel 检查会直接报出来：")
	fmt.Println("    the cancel function returned by context.WithTimeout should be called,")
	fmt.Println("    not discarded, to avoid a context leak")
	fmt.Println()
	fmt.Println("  ⭐ 规则很简单：只要拿到 cancel，下一行就 defer 掉它。")
}

func main() {
	demoDoneIsChannel()
	demoPropagation()
	demoDeadline()
	demoErrKinds()
	demoCooperative()
	demoValue()
	demoLostCancel()
}
