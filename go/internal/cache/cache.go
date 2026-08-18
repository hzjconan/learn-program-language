// Package cache 提供一个带 TTL（存活时间）的内存缓存。
//
// 这是 D6 的主练习，但和前几天反过来：**实现是我写的，测试是你写的**。
//
// # 你的任务
//
// 这个文件里有 **3 个 bug**。配套的 cache_test.go 覆盖率是 100%，
// 但一个都没抓到 —— 这就是 D6 §5 那句「覆盖率量的是代码被执行过，不是代码是对的」。
//
// 三个 bug 分属三个类别：
//
//  1. **时间相关的逻辑错误** —— 需要注入假时钟（见下面 Cache.now 字段）
//  2. **统计口径错误** —— 需要构造「部分数据已过期」的状态再断言
//  3. **并发安全问题** —— 需要并发测试 + go test -race
//
// 在 cache_bugs_test.go 里写测试让它们暴露（先红），再回来修这个文件（转绿）。
//
// # 规格说明就是下面的文档注释
//
// 每个方法的注释描述的是**正确行为**。bug 全是「实现和文档不符」，
// 不存在「文档没写清楚所以两种理解都行」的情况。有歧义算我输，来问我。
package cache

import (
	"sync"
	"time"
)

// entry 是缓存里的一条记录。
type entry[V any] struct {
	val       V
	expiresAt time.Time
	hits      int
}

// Cache 是一个带 TTL 的键值缓存，所有条目共用同一个 TTL。
//
// 必须用 New 构造：零值不可用（items 是 nil map，now 是 nil 函数）。
// 这和我们前几天强调的「零值可用」相反 —— 因为 TTL 没有合理的默认值，
// 强迫调用方显式指定比悄悄用一个 0 更安全。
//
// # 并发安全性
//
// **安全。** 所有导出方法都可以被多个 goroutine 并发调用。
// （这是文档承诺的行为。它现在做到了吗？这是 bug 3 要问的。）
type Cache[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]entry[V]
	ttl   time.Duration

	// now 返回当前时间。生产代码里它就是 time.Now。
	//
	// ⭐ 这是留给测试的口子：白盒测试（package cache）可以替换它，
	// 让时间瞬间往前跳，而不用真的 time.Sleep 几秒钟。
	//
	//	c := New[string, int](time.Minute)
	//	fake := time.Now()
	//	c.now = func() time.Time { return fake }
	//	...
	//	fake = fake.Add(2 * time.Minute)   // 时间前进两分钟
	//
	// 「把时间变成依赖」是可测代码的一个通用技巧，D10 讲 context 超时时还会用到。
	now func() time.Time
}

// New 创建一个 TTL 为 ttl 的缓存。
//
// ttl <= 0 时，所有条目立即过期（等价于禁用缓存）。
func New[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{
		items: make(map[K]entry[V]),
		ttl:   ttl,
		now:   time.Now,
	}
}

// Set 写入一个键值对，并把它的过期时刻重置为「现在 + ttl」。
//
// 同 key 覆盖，命中计数（见 Hits）随之清零。
func (c *Cache[K, V]) Set(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[k] = entry[V]{
		val:       v,
		expiresAt: c.now().Add(c.ttl),
	}
}

// Get 返回 k 对应的值。
//
// **已过期的键视为不存在**，返回零值和 false —— 即使它还没被 Cleanup 清掉。
// 「过期」的定义：当前时刻 >= 过期时刻（正好到期的那一瞬间算已过期）。
//
// 每次成功命中会让该条目的命中计数加一（见 Hits）。
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[k]
	if !ok || !e.expiresAt.After(c.now()) {
		var zero V
		return zero, false
	}

	e.hits++
	c.items[k] = e

	return e.val, true
}

// Hits 返回 k 被成功命中过多少次；k 不存在时返回 0。
func (c *Cache[K, V]) Hits(k K) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.items[k].hits
}

// Delete 删除 k，k 不存在时无副作用。
func (c *Cache[K, V]) Delete(k K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, k)
}

// Len 返回**未过期**的条目数。
//
// 已过期但还没被 Cleanup 清掉的条目**不计入** —— 因为从调用方的角度看，
// 它们已经取不出来了（见 Get），把它们算进来会让 Len 和实际可用的数据对不上。
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, e := range c.items {
		if e.expiresAt.After(c.now()) {
			count++
		}
	}
	return count
}

// Cleanup 删除所有已过期的条目，返回删掉的条数。
//
// 过期判定和 Get 一致：当前时刻 >= 过期时刻即为已过期。
func (c *Cache[K, V]) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	now := c.now()
	for k, e := range c.items {
		if !now.Before(e.expiresAt) {
			delete(c.items, k)
			n++
		}
	}
	return n
}
