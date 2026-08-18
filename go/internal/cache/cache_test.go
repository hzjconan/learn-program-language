package cache

import (
	"testing"
	"time"
)

// 这份测试文件是我写的，**别改它**。
//
// 它的覆盖率是 100.0% —— cache.go 里每一条语句都被执行到了。
// 但它一个 bug 都抓不到。你的工作在 cache_bugs_test.go 里。
//
// 读的时候顺便想一个问题：**这些断言分别在验证什么？**
// 你会发现它们验证的都是「最顺的那条路」，而 bug 从来不长在最顺的路上。

func TestSetGet(t *testing.T) {
	c := New[string, int](time.Minute)
	c.Set("a", 1)

	got, ok := c.Get("a")
	if !ok {
		t.Fatal("Get(\"a\") 没找到")
	}
	if got != 1 {
		t.Errorf("Get(\"a\") = %d, want 1", got)
	}
}

func TestGetMissing(t *testing.T) {
	c := New[string, int](time.Minute)

	got, ok := c.Get("不存在")
	if ok {
		t.Errorf("Get(不存在的键) 返回了 ok=true")
	}
	if got != 0 {
		t.Errorf("Get(不存在的键) = %d, want 0（零值）", got)
	}
}

func TestSetOverwrite(t *testing.T) {
	c := New[string, int](time.Minute)
	c.Set("a", 1)
	c.Set("a", 2)

	got, _ := c.Get("a")
	if got != 2 {
		t.Errorf("覆盖后 Get(\"a\") = %d, want 2", got)
	}
}

func TestDelete(t *testing.T) {
	c := New[string, int](time.Minute)
	c.Set("a", 1)
	c.Delete("a")

	if _, ok := c.Get("a"); ok {
		t.Error("Delete 之后还能 Get 到")
	}

	c.Delete("不存在") // 不该 panic
}

func TestLen(t *testing.T) {
	c := New[string, int](time.Minute)
	if n := c.Len(); n != 0 {
		t.Errorf("空缓存 Len() = %d, want 0", n)
	}

	c.Set("a", 1)
	c.Set("b", 2)
	if n := c.Len(); n != 2 {
		t.Errorf("Len() = %d, want 2", n)
	}
}

func TestHits(t *testing.T) {
	c := New[string, int](time.Minute)
	c.Set("a", 1)

	c.Get("a")
	c.Get("a")
	c.Get("a")

	if n := c.Hits("a"); n != 3 {
		t.Errorf("Hits(\"a\") = %d, want 3", n)
	}
	if n := c.Hits("不存在"); n != 0 {
		t.Errorf("Hits(不存在的键) = %d, want 0", n)
	}
}

func TestCleanup(t *testing.T) {
	c := New[string, int](20 * time.Millisecond)

	c.Set("旧的", 1)
	time.Sleep(30 * time.Millisecond) // 等它过期
	c.Set("新的", 2)

	if n := c.Cleanup(); n != 1 {
		t.Errorf("Cleanup() = %d, want 1", n)
	}
}
