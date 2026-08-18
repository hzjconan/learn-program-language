package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBugs_SetResetExistingTTL(t *testing.T) {
	ttl := time.Minute
	c, advance := newTestCache(t, ttl)

	c.Set("a", 1)

	now := advance(time.Second)

	c.Set("a", 2)
	e2 := c.items["a"]

	if !e2.expiresAt.Equal(now.Add(ttl)) {
		t.Errorf("Set 后 expiredAt = %v, 期望 %v (差 %v)", e2.expiresAt, now.Add(ttl), e2.expiresAt.Sub(now.Add(ttl)))
	}
}

func TestBugs_NewWithNegativeTTL(t *testing.T) {
	c, _ := newTestCache(t, -time.Minute)
	if c.ttl != -time.Minute {
		t.Errorf("New(-time.Minute) 的 ttl = %v, 期望 %v", c.ttl, -time.Minute)
	}

	c.Set("a", 1)
	if _, ok := c.Get("a"); ok {
		t.Errorf("New(-time.Minute) 下，Get(\"a\") 返回了 ok=true, 期望 ok=false")
	}
}

func TestBugs_GetExpiredKey(t *testing.T) {
	c, advance := newTestCache(t, time.Minute)

	c.Set("a", 1)

	// 一分钟后正好到期
	_ = advance(time.Minute)

	if _, ok := c.Get("a"); ok {
		t.Errorf("Get(\"a\") 返回了 ok=true, 期望 ok=false")
	}
}

func TestBugs_LenWithNegativeTTL(t *testing.T) {
	c, _ := newTestCache(t, -time.Minute)

	c.Set("a", 1)

	if c.Len() != 0 {
		t.Errorf("New(-time.Minute) 下，Len() = %d, 期望 0", c.Len())
	}
}

func TestBugs_LenWithExpiredData(t *testing.T) {
	c, advance := newTestCache(t, time.Minute)

	c.Set("a", 1)
	c.Set("b", 1)

	// 一分钟后a应该正好到期
	_ = advance(time.Minute)

	c.Set("c", 1)

	if c.Len() != 1 {
		t.Errorf("Len() = %d, 期望 1", c.Len())
	}
}

func TestBugs_ConcurrentAccess(t *testing.T) {
	c, _ := newTestCache(t, time.Minute)

	// 先准备状态：让后面的并发调用能真正命中
	for i := range 10 {
		c.Set(fmt.Sprintf("k%d", i), i)
	}

	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for range 100 {
				switch g % 3 { // 不同 goroutine
				case 0:
					c.Get("k0")
					c.Hits("k0")
				case 1:
					c.Set("k0", 1)
					c.Set("x0", 1)
				case 2:
					c.Delete("k9")
					c.Delete("x0")
					c.Len()
					c.Cleanup()
				}
			}
		})
	}
	wg.Wait()
}

func newTestCache(t *testing.T, ttl time.Duration) (*Cache[string, int], func(time.Duration) time.Time) {
	t.Helper()

	c := New[string, int](ttl)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // 固定基准，结果可复现
	c.now = func() time.Time { return now }

	advance := func(d time.Duration) time.Time {
		now = now.Add(d)
		return now
	}
	return c, advance
}
