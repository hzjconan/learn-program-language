package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// 测试文件是我写的，别改。
// 你自己想补的用例写在 httpx_extra_test.go 里，函数名用 TestHTTPX_Extra_Xxx。

// req 造一个带 ctx 的测试请求。
func req(method, path string, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	}
	return r
}

// okHandler 是个什么都不做、只回 200 的 handler。
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok") //nolint:errcheck // 测试
})

// ---------- Chain ----------

func TestHTTPX_ChainOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string

	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				order = append(order, name+"进")
				mu.Unlock()
				next.ServeHTTP(w, r)
				mu.Lock()
				order = append(order, name+"出")
				mu.Unlock()
			})
		}
	}

	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
	}), mark("A"), mark("B"), mark("C"))

	h.ServeHTTP(httptest.NewRecorder(), req("GET", "/", ""))

	want := "A进 B进 C进 handler C出 B出 A出"
	if got := strings.Join(order, " "); got != want {
		t.Errorf("执行顺序 = %q\nwant %q\n（Chain 的约定是【从左到右 = 从外到内】—— 提示：倒着遍历）",
			got, want)
	}
}

func TestHTTPX_ChainEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	Chain(okHandler).ServeHTTP(rec, req("GET", "/", ""))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("没有中间件时应该原样调用 handler，得到 %d %q", rec.Code, rec.Body.String())
	}
}

// ---------- RequestID ----------

func TestHTTPX_RequestIDGenerates(t *testing.T) {
	var got string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := RequestIDFrom(r.Context())
		if !ok {
			t.Error("ctx 里取不到 request ID")
		}
		got = id
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/", ""))

	if got == "" {
		t.Fatal("生成的 request ID 是空的")
	}
	if h := rec.Header().Get(RequestIDHeader); h != got {
		t.Errorf("响应头 %s = %q, want %q（ctx 里和响应头里应该是同一个）",
			RequestIDHeader, h, got)
	}
}

// TestHTTPX_RequestIDReuses 验证链路追踪：上游给了 ID 就复用，不要另生成。
func TestHTTPX_RequestIDReuses(t *testing.T) {
	const upstream = "upstream-trace-abc123"
	var got string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = RequestIDFrom(r.Context())
	}))

	r := req("GET", "/", "")
	r.Header.Set(RequestIDHeader, upstream)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if got != upstream {
		t.Errorf("ctx 里的 ID = %q, want %q（请求头里已有就要复用，跨服务要保持同一个）", got, upstream)
	}
	if h := rec.Header().Get(RequestIDHeader); h != upstream {
		t.Errorf("响应头 = %q, want %q", h, upstream)
	}
}

func TestHTTPX_RequestIDUnique(t *testing.T) {
	seen := map[string]bool{}
	var mu sync.Mutex
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := RequestIDFrom(r.Context())
		mu.Lock()
		defer mu.Unlock()
		if seen[id] {
			t.Errorf("ID %q 重复了", id)
		}
		seen[id] = true
	}))

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() { h.ServeHTTP(httptest.NewRecorder(), req("GET", "/", "")) })
	}
	wg.Wait()

	if len(seen) != 100 {
		t.Errorf("100 个并发请求只生成了 %d 个不同的 ID", len(seen))
	}
}

func TestHTTPX_RequestIDFromEmptyCtx(t *testing.T) {
	if _, ok := RequestIDFrom(context.Background()); ok {
		t.Error("空 ctx 里不该取到 request ID")
	}
}

// ---------- Recover ----------

func TestHTTPX_RecoverCatchesPanic(t *testing.T) {
	var (
		gotRecovered any
		gotStack     []byte
	)
	mw := Recover(func(r *http.Request, recovered any, stack []byte) {
		gotRecovered = recovered
		gotStack = stack
	})

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("handler 炸了")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/boom", "")) // 不该 panic 出来

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("状态码 = %d, want 500", rec.Code)
	}
	if gotRecovered != "handler 炸了" {
		t.Errorf("onPanic 收到的 recovered = %v, want %q", gotRecovered, "handler 炸了")
	}
	if len(gotStack) == 0 {
		t.Error("onPanic 没收到堆栈 —— 用 debug.Stack()")
	}
	if !strings.Contains(string(gotStack), "goroutine") {
		t.Errorf("堆栈看起来不对：%.100s", gotStack)
	}
}

func TestHTTPX_RecoverNilHandler(t *testing.T) {
	h := Recover(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("炸了")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/", "")) // onPanic 是 nil 也不能崩
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("状态码 = %d, want 500", rec.Code)
	}
}

func TestHTTPX_RecoverPassesThrough(t *testing.T) {
	h := Recover(nil)(okHandler)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/", ""))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("不 panic 时应该原样放行，得到 %d %q", rec.Code, rec.Body.String())
	}
}

// TestHTTPX_RecoverIgnoresAbortHandler 验证 http.ErrAbortHandler 要原样传出去。
//
// 这是标准库用来「主动放弃这个请求」的哨兵 panic，惯例是不拦它。
func TestHTTPX_RecoverIgnoresAbortHandler(t *testing.T) {
	h := Recover(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		v := recover()
		if v == nil {
			t.Error("http.ErrAbortHandler 应该被原样 panic 出去，不该被 Recover 拦下")
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), req("GET", "/", ""))
}

// ---------- Logging ----------

func TestHTTPX_LoggingRecordsStatus(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
		wantBytes  int
	}{
		{
			name:       "显式 200",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); fmt.Fprint(w, "hi") }, //nolint:errcheck
			wantStatus: 200, wantBytes: 2,
		},
		{
			name:       "隐式 200（只 Write 不 WriteHeader）",
			handler:    func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello") }, //nolint:errcheck
			wantStatus: 200, wantBytes: 5,
		},
		{
			name:       "404",
			handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) },
			wantStatus: 404, wantBytes: 0,
		},
		{
			name:       "500",
			handler:    func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) },
			wantStatus: 500, wantBytes: 5, // "boom\n"
		},
		{
			name:       "什么都不写",
			handler:    func(w http.ResponseWriter, r *http.Request) {},
			wantStatus: 200, wantBytes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(WithRequestID(slog.NewJSONHandler(&buf, nil)))
			h := Logging(logger)(tt.handler)
			h.ServeHTTP(httptest.NewRecorder(), req("POST", "/some/path", ""))

			lines := parseLines(t, &buf)
			if lines[0]["status"] != float64(tt.wantStatus) {
				t.Errorf("Status = %v, want %d", lines[0]["status"], tt.wantStatus)
			}
			if lines[0]["bytes"] != float64(tt.wantBytes) {
				t.Errorf("Bytes = %v, want %d", lines[0]["bytes"], tt.wantBytes)
			}
			if lines[0]["method"] != "POST" {
				t.Errorf("Method = %v, want POST", lines[0]["method"])
			}
			if lines[0]["path"] != "/some/path" {
				t.Errorf("Path = %v, want /some/path", lines[0]["path"])
			}
			if d, ok := lines[0]["duration_ms"].(float64); !ok || d <= 0 {
				t.Errorf("Duration(ms) = %v, 应该是正数", lines[0]["duration_ms"])
			}
		})
	}
}

// TestHTTPX_LoggingPicksUpRequestID 验证 Logging 能拿到 RequestID 放进 ctx 的值。
func TestHTTPX_LoggingPicksUpRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(WithRequestID(slog.NewJSONHandler(&buf, nil)))
	// ⭐ RequestID 必须在【外】层 —— 它用 r.WithContext 造了个新 request 传给内层，
	// 反过来的话 Logging 手上还是老的 r，ctx 里没有 ID。
	h := Chain(okHandler, RequestID, Logging(logger))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/", ""))

	lines := parseLines(t, &buf)
	id, present := lines[0]["request_id"]
	if !present || id == "" {
		t.Error("LogEntry.RequestID 是空的 —— RequestID 在外层，Logging 应该能从 ctx 拿到")
	}
	if id != rec.Header().Get(RequestIDHeader) {
		t.Errorf("日志里的 ID = %v，响应头里的 = %v，应该一致",
			id, rec.Header().Get(RequestIDHeader))
	}
}

func TestHTTPX_LoggingNoRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(WithRequestID(slog.NewJSONHandler(&buf, nil)))
	h := Logging(logger)(okHandler) // 没有 RequestID 中间件
	h.ServeHTTP(httptest.NewRecorder(), req("GET", "/", ""))

	lines := parseLines(t, &buf)
	id, present := lines[0]["request_id"]
	if present && id != "" {
		t.Errorf("没有 RequestID 中间件时应该留空，得到 %v", id)
	}
}

// ---------- RateLimit ----------

func TestHTTPX_RateLimitBurst(t *testing.T) {
	h := RateLimit(3, time.Hour)(okHandler) // 桶容量 3，一小时才补一个

	var codes []int
	for range 5 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req("GET", "/", ""))
		codes = append(codes, rec.Code)
	}

	want := []int{200, 200, 200, 429, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Errorf("第 %d 个请求 = %d, want %d（容量 3，前三个放行，之后 429）\n实际全部: %v",
				i+1, codes[i], want[i], codes)
			break
		}
	}
}

func TestHTTPX_RateLimitRetryAfter(t *testing.T) {
	h := RateLimit(1, time.Hour)(okHandler)
	h.ServeHTTP(httptest.NewRecorder(), req("GET", "/", "")) // 用掉唯一的令牌

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req("GET", "/", ""))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("状态码 = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 响应应该带 Retry-After 头")
	}
}

// TestHTTPX_RateLimitRefills 验证令牌会随时间补充。
func TestHTTPX_RateLimitRefills(t *testing.T) {
	h := RateLimit(1, 20*time.Millisecond)(okHandler)

	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req("GET", "/", ""))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req("GET", "/", ""))
	if rec1.Code != 200 || rec2.Code != 429 {
		t.Fatalf("前两个请求 = %d %d, want 200 429", rec1.Code, rec2.Code)
	}

	time.Sleep(40 * time.Millisecond) // 等它补充

	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req("GET", "/", ""))
	if rec3.Code != 200 {
		t.Errorf("等了 40ms（refill=20ms）之后 = %d, want 200 —— 令牌没补充？", rec3.Code)
	}
}

// TestHTTPX_RateLimitConcurrent 验证并发安全 + 总放行数不超过容量。
func TestHTTPX_RateLimitConcurrent(t *testing.T) {
	const capacity = 50
	h := RateLimit(capacity, time.Hour)(okHandler)

	var allowed, rejected int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 300 {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req("GET", "/", ""))
			mu.Lock()
			defer mu.Unlock()
			if rec.Code == http.StatusOK {
				allowed++
			} else {
				rejected++
			}
		})
	}
	wg.Wait()

	if allowed != capacity {
		t.Errorf("放行了 %d 个，want 恰好 %d（多了说明计数有竞态，少了说明多扣了令牌）",
			allowed, capacity)
	}
	if allowed+rejected != 300 {
		t.Errorf("总数 = %d, want 300", allowed+rejected)
	}
}

// parseLines 把 slog 的 JSON 输出按行解析成 map，供断言使用。
//
// slog 的 JSONHandler 每条日志写一行（JSON Lines 格式），所以按 \n 切开
// 再逐行 Unmarshal 就行。
func parseLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper() // ⭐ 失败时报错指向【调用方】那一行，不是这里（D6）

	var out []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if ln == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			// 日志不是合法 JSON 属于「测试前提坏了」，直接 Fatal
			t.Fatalf("日志不是合法 JSON: %v\n%s", err, ln)
		}
		out = append(out, m)
	}
	return out
}

// ---------- 整链 ----------

// TestHTTPX_FullChain 验证四个中间件串起来能正常工作。
func TestHTTPX_FullChain(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(WithRequestID(slog.NewJSONHandler(&buf, nil)))

	var mu sync.Mutex
	var panics int

	// ⭐ 顺序（从左到右 = 从外到内）：
	//   RequestID 必须在【最外】—— r.WithContext 造的新 request 只传给内层，
	//                            放内层的话 Logging / Recover 都拿不到 ID
	//   Logging   第二        —— 要记录包括限流拒绝在内的所有请求和真实总耗时
	//   Recover   第三        —— 在 Logging 内层，这样它写的 500 能被记进日志
	//   RateLimit 第四        —— 在业务之前拒绝，省下后面的开销
	h := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/panic" {
				panic("故意的")
			}
			fmt.Fprint(w, "ok") //nolint:errcheck
		}),
		RequestID,
		Logging(logger),
		Recover(func(r *http.Request, v any, s []byte) { mu.Lock(); panics++; mu.Unlock() }),
		RateLimit(100, time.Hour),
	)

	for _, p := range []string{"/ok", "/panic", "/ok"} {
		h.ServeHTTP(httptest.NewRecorder(), req("GET", p, ""))
	}

	lines := parseLines(t, &buf)
	if len(lines) != 3 {
		t.Fatalf("记录了 %d 条日志, want 3", len(lines))
	}
	// ⚠️ JSON 数字是 float64（D12 §1.1）
	if lines[1]["status"] != float64(500) {
		t.Errorf("panic 那次记录的状态码 = %v, want 500\n"+
			"（Logging 在 Recover 外层，应该看到 Recover 写的 500）", lines[1]["status"])
	}
	if panics != 1 {
		t.Errorf("onPanic 被调用了 %d 次, want 1", panics)
	}
	for i, ln := range lines {
		id, present := ln["request_id"]
		if !present || id == "" {
			t.Errorf("第 %d 条日志没有 request ID", i)
		}
	}
}

// TestHTTPX_WithRequestIDSurvivesWith 锁住 QA #18：
// ctxHandler 必须重写 WithAttrs / WithGroup 并把结果【重新包一层】，
// 否则一调 .With() 包装层就脱落，request_id 静默丢失。
//
// ⚠️ 这三个用例都会因为「删掉那两个方法」而失败 —— 这正是它们存在的理由。
func TestHTTPX_WithRequestIDSurvivesWith(t *testing.T) {
	newLogger := func(buf *bytes.Buffer) slog.Handler {
		return WithRequestID(slog.NewJSONHandler(buf, nil))
	}
	ctx := context.WithValue(context.Background(), reqIDKey, "req-1")

	t.Run("WithAttrs 之后仍在顶层", func(t *testing.T) {
		var buf bytes.Buffer
		h := newLogger(&buf).WithAttrs([]slog.Attr{slog.String("component", "http")})
		slog.New(h).InfoContext(ctx, "x")

		ln := parseLines(t, &buf)[0]
		if ln["request_id"] != "req-1" {
			t.Errorf("request_id = %v, want req-1\n"+
				"（ctxHandler.WithAttrs 没重写？包装层脱落了）", ln["request_id"])
		}
	})

	t.Run("WithGroup 之后嵌在组里", func(t *testing.T) {
		var buf bytes.Buffer
		h := newLogger(&buf).WithGroup("g")
		slog.New(h).InfoContext(ctx, "x")

		ln := parseLines(t, &buf)[0]
		// ⚠️ WithGroup 之后，Handle 里 AddAttrs 加的属性也会进这个组
		g, ok := ln["g"].(map[string]any)
		if !ok {
			t.Fatalf("没有 g 这个分组: %v", ln)
		}
		if g["request_id"] != "req-1" {
			t.Errorf("g.request_id = %v, want req-1\n"+
				"（ctxHandler.WithGroup 没重写？）", g["request_id"])
		}
	})

	t.Run("Logger.With 也走这条路", func(t *testing.T) {
		var buf bytes.Buffer
		// ⭐ 这是真实用法：logger.With(...) 内部调的就是 handler.WithAttrs
		slog.New(newLogger(&buf)).With("component", "http").InfoContext(ctx, "x")

		ln := parseLines(t, &buf)[0]
		if ln["request_id"] != "req-1" {
			t.Errorf("request_id = %v, want req-1", ln["request_id"])
		}
	})
}
