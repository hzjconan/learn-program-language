// Package pipeline 是 D10 的主练习：一条可取消的多阶段流水线。
//
// # 结构
//
//	Source ──→ ┌→ Stage1 ─┐
//	           ├→ Stage2 ─┼──→ Merge ──→ Run 收集并排序
//	           └→ Stage3 ─┘
//	           （fan-out）    （fan-in）
//
// # 每条 goroutine 都必须有【两个】退出路径
//
//  1. **上游关闭** —— `for v := range in` 自然结束
//  2. **ctx 取消** —— `select` 里的 `<-ctx.Done()` 分支
//
// 只有第 1 条的话，下游提前不读了，这条 goroutine 会永远卡在 `out <- v` 上 —— 泄漏（D8 §7）。
// 只有第 2 条的话，正常路径下 channel 永远不关，下游的 for range 退不出去 —— 死锁。
//
// # 每个 channel 谁关
//
// ⭐ **谁创建谁关闭。** 每个函数创建自己的 out channel，就负责 `defer close(out)`。
// 绝不去关别人给你的 in channel（方向类型 `<-chan int` 会让编译器帮你挡住，D8）。
//
// # 测试会验什么
//
//   - 取消之后所有 goroutine 干净退出（比对 runtime.NumGoroutine()）
//   - 区分主动取消（context.Canceled）和超时（context.DeadlineExceeded）
//   - -race 干净
//   - 并发上限真的生效
//   - 正常路径结果正确且顺序确定
package pipeline

import (
	"context"
	"slices"
	"sync"
)

// Source 把 items 逐个送进返回的 channel。
//
// TODO(D10)：实现我。
//
// 要求：
//   - 送完所有元素后**关闭** channel（否则下游的 for range 退不出去）
//   - ctx 取消时立刻停止发送并关闭 channel
//   - items 为空时返回一个已关闭的 channel（不是 nil —— nil channel 读写永远阻塞，D8 §5）
//
// 提示：无缓冲 channel + select。想想为什么发送也要放进 select。
func Source(ctx context.Context, items []int) <-chan int {
	out := make(chan int)
	if len(items) == 0 {
		close(out)
		return out
	}
	go func() {
		defer close(out)
		for _, v := range items {
			select {
			case <-ctx.Done():
				return
			case out <- v:
			}
		}
	}()

	return out
}

// Stage 是一个加工阶段：从 in 读，用 f 加工，写到返回的 channel。
//
// TODO(D10)：实现我。
//
// 要求：
//   - 两个退出路径都要有（见包注释）
//   - 自己创建的 out channel 自己关
//   - **不要**关 in
//
// 注意 f 可能很慢。ctx 在 f 执行【期间】取消的话，这个阶段应该在 f 返回后尽快退出 ——
// 我们没法中断 f 本身（D10 §5：取消停不掉任何东西）。
func Stage(ctx context.Context, in <-chan int, f func(int) int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)
		for v := range in {
			select { // ⭐ 算之前先看一眼该不该继续
			case <-ctx.Done():
				return
			default:
			}

			r := f(v)

			select { // ⭐ 算之后先看一眼该不该继续
			case out <- r:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Merge 把多个 channel 汇成一个（fan-in）。
//
// TODO(D10)：实现我。
//
// 要求：
//   - **所有** ins 都关闭之后，才关闭 out（提示：WaitGroup + 单独一条 goroutine，D8 §8）
//   - ctx 取消时所有搬运 goroutine 都要能退出
//   - ins 为空时返回一个已关闭的 channel
//
// ⚠️ 这是最容易泄漏的一个函数：N 条搬运 goroutine，每条都要有两个退出路径。
func Merge(ctx context.Context, ins ...<-chan int) <-chan int {
	out := make(chan int)
	if len(ins) == 0 {
		close(out)
		return out
	}

	var wg sync.WaitGroup

	for _, in := range ins {
		wg.Go(func() {
			for v := range in {
				select {
				case <-ctx.Done():
					return
				case out <- v:
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// Run 组装完整流水线：Source → fan-out 到 workers 个 Stage → Merge → 收集。
//
// TODO(D10)：实现我。
//
// 返回值：
//   - 正常完成 → 结果**升序排好**的切片 + nil
//     （并发的完成顺序是随机的，不排序测试会时红时绿 —— D6）
//   - ctx 取消或超时 → nil + ctx.Err()
//     调用方要能用 errors.Is 区分 context.Canceled 和 context.DeadlineExceeded
//   - items 为空 → nil + nil
//   - workers <= 0 → 当作 1
//
// ⚠️ **取消时必须保证所有 goroutine 都退出**，别直接 return 就走人 ——
// 那些还在 Source/Stage/Merge 里的 goroutine 会卡在发送上，永远不退（泄漏）。
//
// 想一想：怎么让它们知道该走了？（提示：它们都拿着同一个 ctx）
func Run(ctx context.Context, items []int, workers int, f func(int) int) ([]int, error) {
	if workers <= 0 {
		workers = 1
	}

	jobs := Source(ctx, items)

	resultsChan := make([]<-chan int, workers)
	for i := range workers {
		resultsChan[i] = Stage(ctx, jobs, f)
	}

	out := Merge(ctx, resultsChan...)

	var results []int
	for v := range out { // out 关闭时自然退出
		results = append(results, v)
	}

	if err := ctx.Err(); err != nil { // 收完了才判断：是正常结束还是被取消
		return nil, err
	}
	slices.Sort(results)
	return results, nil
}
