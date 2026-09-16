package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------- Unless ----------

func TestHTTPX_Extra_UnlessAppliesWhenNotSkipped(t *testing.T) {
	// 用 RateLimit 作为被测 middleware：容量 1，第二次请求应该 429
	h := Unless(RateLimit(1, time.Hour), PathIs("/skip"))(okHandler)

	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req("GET", "/api", ""))
	if rec1.Code != http.StatusOK {
		t.Fatalf("第一次 /api = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req("GET", "/api", ""))
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("第二次 /api = %d, want 429（middleware 应该生效）", rec2.Code)
	}
}

func TestHTTPX_Extra_UnlessBypassesWhenSkipped(t *testing.T) {
	h := Unless(RateLimit(1, time.Hour), PathIs("/skip"))(okHandler)

	// /skip 被跳过 —— 不管来多少次都放行
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req("GET", "/skip", ""))
		if rec.Code != http.StatusOK {
			t.Errorf("第 %d 次 /skip = %d, want 200（skip 命中，middleware 应该不生效）",
				i+1, rec.Code)
		}
	}
}

func TestHTTPX_Extra_UnlessMixedPaths(t *testing.T) {
	// 混合场景：同一个 handler，/skip 不限流，/hit 限流
	h := Unless(RateLimit(1, time.Hour), PathIs("/skip"))(okHandler)

	// 第一次请求（不管路径）会消耗令牌
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req("GET", "/hit", ""))
	if rec1.Code != http.StatusOK {
		t.Fatalf("第一次 /hit = %d, want 200", rec1.Code)
	}

	// /hit 第二次应该被限流
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req("GET", "/hit", ""))
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("第二次 /hit = %d, want 429", rec2.Code)
	}

	// /skip 绕过限流 —— 令牌已空也应该放行
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req("GET", "/skip", ""))
	if rec3.Code != http.StatusOK {
		t.Errorf("/skip = %d, want 200（应该被 skip 绕过）", rec3.Code)
	}
}

// TestHTTPX_UnlessNilSkip 验证 skip 为 nil 时的行为（不 skip，全走 middleware）。
func TestHTTPX_Extra_UnlessNilSkip(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("skip 为 nil 时应该 panic —— 调用方的责任")
		}
	}()
	Unless(RateLimit(1, time.Hour), nil)(okHandler).ServeHTTP(
		httptest.NewRecorder(), req("GET", "/", ""))
}

// ---------- PathIs ----------

func TestHTTPX_Extra_PathIsMatches(t *testing.T) {
	skip := PathIs("/healthz", "/readyz")

	for _, p := range []string{"/healthz", "/readyz"} {
		r := req("GET", p, "")
		if !skip(r) {
			t.Errorf("PathIs(%q) = false, want true", p)
		}
	}
}

func TestHTTPX_Extra_PathIsDoesNotMatch(t *testing.T) {
	skip := PathIs("/healthz", "/readyz")

	for _, p := range []string{"/", "/api", "/healthz/details", "/ready"} {
		r := req("GET", p, "")
		if skip(r) {
			t.Errorf("PathIs(%q) = true, want false（精确匹配，不做前缀/子串匹配）", p)
		}
	}
}

func TestHTTPX_Extra_PathIsEmptyList(t *testing.T) {
	skip := PathIs()

	for _, p := range []string{"/", "/healthz", "/anything"} {
		r := req("GET", p, "")
		if skip(r) {
			t.Errorf("PathIs() 对 %q 返回 true, want false（空列表 = 全不匹配）", p)
		}
	}
}

// rate limit
func TestHTTPX_Extra_RateLimitRefillsProportionally(t *testing.T) {
	// capacity=10, refill=100ms → 正确实现每 10ms 补 1 个；bug 实现100ms 才补 1 个
	h := RateLimit(10, 100*time.Millisecond)(okHandler)

	for range 10 { // 耗光
		h.ServeHTTP(httptest.NewRecorder(), req("GET", "/", ""))
	}
	time.Sleep(50 * time.Millisecond) // 应该补回约 5 个

	allowed := 0
	for range 10 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req("GET", "/", ""))
		if rec.Code == 200 {
			allowed++
		}
	}
	// 正确：≈5（留点余量，3~7）；bug：0
	if allowed < 3 || allowed > 7 {
		t.Fatalf("等 50ms 后放行了 %d 个, want ≈5 —— 补充速率是 c 还是 1/refill？", allowed)
	}
}
