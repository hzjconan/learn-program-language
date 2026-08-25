// Package crawl 是一个逐层推进、并发受限的爬取器，D8 的主练习。
//
// # 为什么抓取行为要抽象成接口
//
// Fetcher 是 D5 §2 的小接口哲学 + 依赖倒置：测试里不用真联网，
// 生产里换成 HTTP 客户端也不用改这个包一行。
//
// # 这个包覆盖的知识点
//
//   - select + 超时，且**不泄漏 goroutine**（D8 §5、§7）
//   - worker pool：close(jobs) / wg.Wait() 单独起 goroutine / close(results)（D8 §8）
//   - 「不要用共享内存来通信」的正确用法：去重表只属于协调者（D8 §9）
//   - 并发返回顺序随机 → 结果必须自己排序，否则测试随机失败（D6）
package crawl

import (
	"cmp"
	"errors"
	"slices"
	"sync"
	"time"
)

// ErrTimeout 表示单次抓取超过了给定时限。
//
// 哨兵错误：调用方用 errors.Is(err, ErrTimeout) 判断（QA #8）。
var ErrTimeout = errors.New("crawl: 抓取超时")

// Fetcher 抓取一个 URL，返回页面里的链接。
//
// 一个方法的小接口 —— 真实实现可以是 HTTP 客户端、本地文件、测试假数据。
type Fetcher interface {
	// Fetch 返回 url 页面里的所有链接。
	Fetch(url string) ([]string, error)
}

// Result 是一次抓取的结果。
//
// 全是值类型和切片 —— 注意 Links 是切片，往外传时要想想别名问题（D3、D7 review）。
type Result struct {
	// URL 是被抓取的地址。
	URL string
	// Depth 是它距离种子的层数，种子本身是 0。
	Depth int
	// Links 是抓到的链接；出错时为 nil。
	Links []string
	// Err 是抓取失败的原因；成功时为 nil。
	Err error
}

// FetchWithTimeout 抓取 url，超过 timeout 就放弃并返回 ErrTimeout。
//
// TODO(D8)：实现我。
//
// ⚠️ **不能泄漏 goroutine** —— 测试会比对调用前后的 runtime.NumGoroutine()。
//
// 这是 D8 §7 那个坑的正面遭遇：你会起一条 goroutine 去调 f.Fetch，
// 然后用 select 等它或者等超时。超时之后**那条 goroutine 还在跑**，
// 它迟早会往 channel 里发结果 —— 如果那时已经没人接收，它就永远卡在那儿了。
//
// 提示：想想 make(chan T) 和 make(chan T, 1) 的区别。
//
// 注意：**我们没法真正「取消」f.Fetch**，它还会继续跑完。
// 我们能做的只是「不再等它」并且「不让它卡住」。真正的取消要靠 context（D10）。
func FetchWithTimeout(f Fetcher, url string, timeout time.Duration) ([]string, error) {
	type fetchR = struct {
		links []string
		err   error
	}
	resultChan := make(chan fetchR, 1)
	go func() {
		links, err := f.Fetch(url)
		resultChan <- fetchR{links, err}
	}()

	select {
	case v := <-resultChan:
		if v.err != nil {
			return nil, v.err
		}
		return v.links, nil
	case <-time.After(timeout):
		return nil, ErrTimeout
	}
}

// FetchAll 用最多 workers 个并发抓取 urls，返回和 urls **一一对应、顺序相同**的结果。
//
// TODO(D8)：实现我。
//
// 要求：
//   - **并发上限必须真的生效**（测试会记录同时在跑的最大数量）
//   - **返回顺序 = 传入顺序**，不是完成顺序（并发完成顺序是随机的）
//   - urls 为空时返回 nil；workers <= 0 时当作 1
//   - 单个 URL 抓取失败不影响其他，把错误记进对应的 Result.Err
//   - 所有 Result 的 Depth 都是 0（这个函数不关心层级）
//
// 提示：worker pool 骨架见 D8 §8。让顺序确定最简单的办法是**把下标一起传进 job**，
// 结果直接写进 `results[i]` —— 每个下标只有一条 goroutine 会写，没有竞态，
// 也就不需要 mutex（这就是「数据有明确的主人」）。
func FetchAll(f Fetcher, urls []string, workers int) []Result {
	if len(urls) == 0 {
		return nil
	}

	type indexedJob struct {
		i   int
		url string
	}
	jobChan := make(chan indexedJob)

	type indexedResult struct {
		i int
		r Result
	}

	resultChan := make(chan indexedResult)

	results := make([]Result, len(urls))

	var wg sync.WaitGroup
	for range max(workers, 1) {
		wg.Go(func() {
			for j := range jobChan {
				links, err := f.Fetch(j.url)
				resultChan <- indexedResult{
					j.i,
					Result{
						URL:   j.url,
						Depth: 0,
						Links: slices.Clone(links),
						Err:   err,
					},
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	go func() {
		for i, url := range urls {
			jobChan <- indexedJob{i, url}
		}
		close(jobChan)
	}()

	for ir := range resultChan {
		results[ir.i] = ir.r
	}

	return results
}

// Crawl 从 seed 出发逐层抓取，最多 maxDepth 层，每层内部最多 workers 个并发。
//
// TODO(D8)：实现我。
//
// 语义：
//   - maxDepth=0 → 只抓 seed 本身
//   - maxDepth=1 → 抓 seed，再抓 seed 里的链接
//   - 每个 URL **只抓一次**（去重）
//   - 抓取失败的 URL 也要出现在结果里（带 Err），但它的链接不再展开
//
// 返回值必须**排好序**：先按 Depth 升序，同层按 URL 字典序升序。
// 否则并发导致的随机顺序会让测试时红时绿（D6）。
//
// # 关于去重表
//
// 用一个普通的 map[string]bool 就行，**不需要 mutex** —— 因为只有协调者
// （也就是这个函数本身）碰它，每层的并发抓取由 FetchAll 内部完成、返回后才更新去重表。
// 这就是 D8 §9 说的「数据有明确的主人」。
//
// 如果你发现自己想给这个 map 加锁，先停下来想想：真的有两条 goroutine 在碰它吗？
func Crawl(f Fetcher, seed string, maxDepth, workers int) []Result {
	if workers <= 0 {
		workers = 1
	}

	if maxDepth < 0 {
		maxDepth = 0
	}

	seen := map[string]bool{seed: true}
	var results []Result
	queue := []string{seed}

	currentDepth := 0

	for currentDepth <= maxDepth {
		rs := FetchAll(f, queue, workers)
		var dedupedLinks []string

		for _, r := range rs {
			r.Depth = currentDepth
			results = append(results, r)
			if r.Err != nil {
				continue
			}

			for _, l := range r.Links {
				if !seen[l] {
					dedupedLinks = append(dedupedLinks, l)
					seen[l] = true
				}
			}

		}
		queue = dedupedLinks

		currentDepth++
	}

	slices.SortFunc(results, func(a, b Result) int {
		if c := cmp.Compare(a.Depth, b.Depth); c != 0 {
			return c
		}
		return cmp.Compare(a.URL, b.URL)
	})

	return results
}
