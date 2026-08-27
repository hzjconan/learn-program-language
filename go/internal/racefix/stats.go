// Package racefix 是 D9 的主练习：一个内存统计服务，里面**埋了 6 个并发问题**。
//
// # 三种不同的暴露方式（这是今天最该体会的）
//
// 同样是「并发出问题」，症状差别巨大：
//
//	直接崩溃   并发写 map → fatal error: concurrent map writes
//	           运行时自带的廉价检查，recover 拦不住，整个进程死
//	数值不对   计数器丢更新 —— 不报错，只是结果小了
//	完全静默   返回内部 map 被调用方改掉、双检锁的可见性问题 ——
//	           跑一万次都可能正常，换台机器/换个负载才炸
//
// **只有 -race 能把它们统一指出来，并告诉你具体是哪两行在冲突。**
//
// # 你的任务
//
//  1. 先跑 `go test ./internal/racefix/`（不带 -race），看它怎么死的
//  2. 再跑 `go test -race ./internal/racefix/`，对比信息量
//  3. 逐个修复，直到 -race 干净、所有数值断言通过
//  4. **每个修复点写一行注释**，说明你选了 mutex / atomic / channel 中的哪个，为什么
//
// 建议顺序：先修让进程崩溃的（问题 2），否则后面的测试根本跑不到。
//
// 📝 本包已完成修复。每个原题问题点的说明保留在对应的方法注释里（搜 "原题问题"），
// 既标出「这里是练习点」，也记录了「当时错在哪、为什么这么修」。
//
// # ⚠️ 有些问题 -race 也报不出来
//
// 竞态检测器只报「无同步的并发内存访问」。它不报：
//   - 死锁（靠 SIGQUIT dump，D8）
//   - 逻辑错误（比如 WaitGroup 用错导致 Wait 提前返回）
//   - 返回内部数据造成的别名（调用方改了你的状态）
//
// 这几类要靠测试的**数值断言**抓。而且 `go vet` 已经先替你抓到了一个 ——
// 跑 `make check` 看看它说什么（工具兜底，D4 SA4005 / D5 SA4023 的老朋友）。
//
// 跑 `go test -race -count=10` 多跑几遍：竞态经常是概率性的。
//
// # 提示
//
// 别为了「消除 -race 报警」而无脑加锁。每处都问一遍 D9 §1 那张表：
// 是单个变量的读改写（atomic）、还是一段临界区（mutex）、还是数据该有个主人（channel）？
package racefix

import (
	"maps"
	"sync"
	"sync/atomic"
	"time"
)

// Counter 是一个并发计数器。
//
// 📝 原题问题 1：n 是裸的 int64，Inc/Value 无任何同步 → 丢更新。
// 修复：改用 atomic.Int64。只是个计数器，单变量的读改写，atomic 比 mutex 简洁也更快。
type Counter struct {
	// 选择用atomic，只是个计数器，比用mutex实现更简洁
	n atomic.Int64
}

// Inc 让计数加一。
func (c *Counter) Inc() {
	c.n.Add(1)
}

// Value 返回当前计数。
func (c *Counter) Value() int64 {
	return c.n.Load()
}

// Registry 记录每个服务名被调用了多少次。
//
// 📝 原题问题 2、3 在下面的 Add 和 Snapshot 里，已修复，各自方法上有说明。
type Registry struct {
	mu sync.RWMutex
	m  map[string]int
}

// NewRegistry 返回一个可用的空 Registry。
func NewRegistry() *Registry {
	return &Registry{m: make(map[string]int)}
}

// Add 给 name 的计数加一。
//
// 📝 原题问题 2：这里原本拿的是 RLock（读锁），但 r.m[name]++ 是【写】。
// 读锁允许多个持有者同时进入 → 并发写 map → fatal error: concurrent map writes。
// 修复：改成 Lock。判据是「方法体里有没有赋值」，不是方法名叫什么。
func (r *Registry) Add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name]++
}

// Count 返回 name 的计数。
func (r *Registry) Count(name string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.m[name]
}

// Snapshot 返回当前所有计数。
//
// 📝 原题问题 3：这里原本直接 return r.m —— 调用方拿到的是内部 map 本身，
// 既能改坏它，也可能在别人写入时遍历它（fatal error: concurrent map iteration and map write）。
// 修复：maps.Clone。值是 int（非引用类型），所以浅拷贝就够；
// 如果值换成 []string，就得逐个深拷贝了（D7 store.Tags、D8 crawl.Links 的同类问题）。
func (r *Registry) Snapshot() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// 这里返回内部map的浅拷贝版，这个map的值是int，不是引用类型，返回浅拷贝完全安全
	return maps.Clone(r.m)
}

// Config 是一份需要惰性加载的配置。
type Config struct {
	// Name 是配置名。
	Name string
	// Retries 是重试次数。
	Retries int
}

var (
	// LoadHit 记录真正执行初始化的次数。测试会检查它必须恰好是 1。
	LoadHit int
)

var load = sync.OnceValue(func() *Config {
	LoadHit++
	time.Sleep(time.Millisecond) // 模拟初始化耗时，放大问题
	return &Config{Name: "prod", Retries: 3}
})

// LoadConfig 惰性加载配置，全局只该加载一次。
//
// 📝 原题问题 4：这里原本是手写的双检锁 —— 第一次 `if !loaded` 的读在锁外面，
// 和锁里面那次写之间没有任何 happens-before，所以可能读到「指针已赋值但字段还没初始化完」。
// 修复：sync.OnceValue。它内部用 atomic + Mutex 正确建立了顺序，而且别人会阻塞等它执行完。
// ⭐ 永远别自己写双检锁 —— 这段在 Java 里不加 volatile 同样是错的。
func LoadConfig() *Config {
	return load()
}

// Sample 是一次采样。
type Sample struct {
	// Service 是服务名。
	Service string
	// Millis 是耗时（毫秒）。
	Millis int
}

// Aggregate 并发处理 samples，返回总耗时和每个服务的调用次数。
//
// 📝 原题问题 5：wg.Add(1) 原本写在 goroutine 【内部】—— Wait 可能在任何一条
// goroutine 执行到 Add 之前就看到计数为 0 而直接放行。go vet 能静态抓到这个。
// 修复：用 wg.Go，它把 Add/go/defer Done 打包，从根上消灭了这类错误。
//
// 📝 原题问题 6：total 是具名返回值，被多条 goroutine 无保护地 += → 丢更新。
// 修复：用局部的 atomic.Int64 累加，最后 Load 出来返回。
// ⚠️ 注意别为了消除竞态就在整个函数体上加一把锁 —— 那会把并发退化成串行，
// TestRacefix_AggregateIsConcurrent 专门抓这个。
//
// 要求（测试会验）：
//   - 返回的 total = 所有 Millis 之和
//   - 返回的 registry 里每个服务的计数正确
//   - 必须真的并发（测试会检查耗时明显小于串行）
func Aggregate(samples []Sample) (int64, *Registry) {
	reg := NewRegistry()
	var wg sync.WaitGroup
	var totalAtomic atomic.Int64

	for _, s := range samples {
		wg.Go(func() {
			time.Sleep(time.Millisecond) // 模拟处理耗时
			totalAtomic.Add(int64(s.Millis))
			reg.Add(s.Service)
		})
	}

	wg.Wait()
	return totalAtomic.Load(), reg
}
