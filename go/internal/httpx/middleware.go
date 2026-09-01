// Package httpx 是 D11 的主练习：不用任何框架，手写一套 HTTP 中间件 + 一个小 API。
//
// # 要实现什么
//
// 四个中间件（签名统一为 func(http.Handler) http.Handler）：
//
//	RequestID   生成/复用请求 ID，放进 ctx 和响应头
//	Recover     兜住 panic，返回 500，进程必须还活着
//	Logging     记录方法/路径/状态码/耗时（要包装 ResponseWriter）
//	RateLimit   令牌桶限流，超限返回 429
//
// 外加 Chain 把它们串起来，以及 store.go 里的 KV API。
//
// # 中间件顺序为什么重要
//
//	Recover    最外层（或仅次于 Logging）—— 要能兜住所有内层的 panic
//	RequestID  靠外 —— 后面的日志都要用它
//	Logging    最外层 —— 要记录真实的总耗时和最终状态码
//	RateLimit  靠外 —— 越早拒绝越省资源
//	Auth       靠内 —— 前面的日志/追踪对未授权请求也该生效
//
// # 测试会验什么
//
//   - Recover 之后进程还活着，且能拿到堆栈
//   - Logging 记录的状态码准确（含 404 / 500 / 429）
//   - 中间件执行顺序可验证
//   - RateLimit 并发安全（-race 干净）
//   - handler 里用了 r.Context()
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// Middleware 是中间件的统一签名：收一个 Handler，返回包装过的 Handler。
type Middleware func(http.Handler) http.Handler

// Chain 把多个中间件串到 h 外面。
//
// TODO(D11)：实现我。
//
// 约定：**从左到右 = 从外到内**。
//
//	Chain(mux, A, B, C)  →  请求先进 A，再 B，再 C，最后到 mux
//
// 提示：倒着遍历。想想为什么正着遍历顺序会反过来。
func Chain(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

// requestIDKey 是 ctx 里存请求 ID 用的 key。
//
// ⭐ 自定义的不导出类型 —— 别的包连写出这个类型都做不到，不可能冲突（D10 §6）。
type requestIDKey struct{}

// 骨架期占位：让 unused linter 知道这个类型是【要用的】。
// 实现 RequestID / RequestIDFrom 之后就可以删掉这一行。
// var _ = requestIDKey{}

var reqIDKey = requestIDKey{}

// RequestIDHeader 是请求 ID 在 HTTP 头里的名字。
const RequestIDHeader = "X-Request-ID"

// RequestIDFrom 从 ctx 里取请求 ID。
//
// TODO(D11)：实现我。
//
// 这就是 D10 §6 说的「类型安全的包装函数」—— 把 ctx.Value 的调用锁在这个包里。
func RequestIDFrom(ctx context.Context) (string, bool) {
	id := ctx.Value(reqIDKey)
	s, ok := id.(string)
	if !ok {
		return "", false
	}
	return s, true
}

// ctxHandler 包住任意 slog.Handler，从 ctx 里取出 request ID 加到每条日志上。
//
// ⭐ 好处是【业务代码什么都不用做】—— 只要用 XxxContext 系列方法，
// 自己打的日志也会自动带上 ID，不用一层层把 ID 传下去。
//
// ⚠️ ctx 是【唯一】的来源，故意不从请求头兜底 —— 理由见 WithRequestID。
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := RequestIDFrom(ctx); ok {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs / WithGroup 必须重写，并且把结果【重新包一层】。
//
// ⚠️ slog.Handler 有四个方法，其中这两个【返回 Handler 自身】。只嵌入不重写的话，
// 它们返回的是【内层那个没被包装的】 handler，包装层当场脱落：
//
//	slog.New(ctxHandler{base})     handler = ctxHandler{base}   ✅
//	    .With("user", "alice")     handler = base               ❌ request_id 没了
//
// 而 logger.With(...) 正是「给每个请求做日志上下文」的推荐用法 ——
// 最该用的写法恰好触发这个 bug，而且【没有任何征兆】，只是日志里静默少一个字段。
//
// 这和 responseRecorder 那个坑同源（嵌入接口 + 只重写关心的方法），
// 但那次至少还有可观察的故障（SSE 不刷新），这次没有。
//
// ⭐ 判据：包装接口时逐个看方法签名，凡是返回该接口自身类型的都要重写并重新包一层。
// Enabled 返回 bool，不涉及包装层，不用重写。
//
// TestHTTPX_WithRequestIDSurvivesWith 锁住这个行为。
func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(name)}
}

// WithRequestID 让 h 产出的每条日志自动带上 ctx 里的 request ID。
//
// 用法（在启动时组装一次）：
//
//	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
//	logger := slog.New(httpx.WithRequestID(base))
//	h := Chain(router, RequestID, Logging(logger), ...)
//
// 两个前提：RequestID 中间件必须在 Logging 【外层】（否则 ctx 里还没 ID），
// 且记日志必须用 XxxContext 系列（普通的 Info 传的是 context.Background()）。
//
// # 为什么【不】从请求头兜底
//
// 改造前 Logging 里有两个来源：① ctx，② 拿不到就读 r.Header 的 X-Request-ID，
// 用来覆盖「没装 RequestID 中间件、但上游网关注入了 ID」的场景。现在只剩 ①。
//
// 这是【刻意】去掉的：
//
//   - RequestID 中间件本身就会复用上游请求头里的 ID（见它的实现），
//     只要装了它，②能覆盖的场景①全都覆盖到了
//   - 唯一剩下的场景是「根本没装 RequestID 中间件」—— 那是【配置错误】。
//     此时日志里没有 request_id 是个【有用的信号】，兜底反而会把错误藏起来
//   - 而且 Handle 只拿得到 ctx，拿不到 *http.Request。要兜底就得让 request ID
//     重新变成两个来源，和「统一由 Handler 注入」的设计打架
//
// ⭐ 一般原则：兜底要看它藏起来的是什么。藏「偶发的缺失」值得，
// 藏「配置错误」不值得 —— 后者你希望它尽早暴露。
func WithRequestID(h slog.Handler) slog.Handler { return ctxHandler{h} }

// genRequestID 生成一个随机 request ID。
func genRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand 几乎不会出错
	return hex.EncodeToString(b)
}

// RequestID 给每个请求分配一个 ID。
//
// TODO(D11)：实现我。
//
// 行为：
//   - 请求头里已经有 X-Request-ID 就**复用**它（链路追踪要求跨服务保持同一个 ID）
//   - 没有就生成一个新的（用 crypto/rand + hex 就行，不用引 uuid 库）
//   - 把 ID 放进 ctx（用 requestIDKey{}）**和**响应头
//
// ⚠️ 往 ctx 里塞值要用 r.WithContext(ctx) 造一个新的 *http.Request —— Request 是不可变的。
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = genRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), reqIDKey, id)))
	})
}

// PanicHandler 在 Recover 兜住 panic 时被调用，用来记日志。
//
// 做成可配置的，是为了测试能断言「堆栈确实被捕获了」。
type PanicHandler func(r *http.Request, recovered any, stack []byte)

// Recover 兜住 handler 里的 panic，返回 500。
//
// TODO(D11)：实现我。
//
// 行为：
//   - panic 之后**进程必须还活着**，且这次请求返回 500
//   - onPanic 不为 nil 时调用它，把 recovered 值和 debug.Stack() 传进去
//   - onPanic 为 nil 时不能崩
//
// ⚠️ 两个细节：
//  1. 如果 handler 在 panic 之前已经写过响应了，你再 WriteHeader(500) 是**无效的**（§3）。
//     这种情况没法补救 —— 但至少要保证进程不挂、日志有记录。
//  2. `http.ErrAbortHandler` 是标准库用来「主动放弃这个请求」的哨兵 panic，
//     惯例是**不拦它，原样再 panic 出去**。
func Recover(onPanic PanicHandler) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err, isError := recovered.(error)
					if isError && errors.Is(err, http.ErrAbortHandler) {
						panic(recovered)
					}
					if onPanic != nil {
						onPanic(r, recovered, debug.Stack())
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					// ⭐ 用包里现成的类型安全包装函数，别自己写 ctx.Value + 断言
					// （拿不到时 reqID 是零值 ""，正好，不用额外判断）
					reqID, _ := RequestIDFrom(r.Context())
					fmt.Fprintf(w, `{"error":"internal server error","request_id":%q}`, reqID) //nolint:errcheck // 响应已崩，写失败也没救
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// responseRecorder 包装 http.ResponseWriter，记录状态码和响应体大小。
// responseRecorder 包装 http.ResponseWriter，把状态码和字节数记下来
// —— ResponseWriter 本身没有「读回状态码」的方法。
//
// # 嵌入接口的代价：可选接口会丢
//
// 这里嵌入的是【接口】http.ResponseWriter，而方法提升看的是嵌入字段的**静态类型**，
// 发生在编译期。编译器只知道这个接口声明的 3 个方法（Header/Write/WriteHeader），
// 所以 Flusher / Hijacker / io.ReaderFrom 全都不会被提升上来（D5 §3 的二元组：
// 运行时那个值确实有 Flush，但编译期看不见）。
//
// 实测一个裸包装一次丢掉 4 个能力，后果是 SSE 失效、WebSocket 升级失败、
// http.ServeFile 用不了 sendfile 零拷贝（功能正常但性能掉档，最难发现）。
//
// 下面两个方法就是补救，各自服务不同的调用方。
type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap 让 http.ResponseController 能顺着包装链找到底层的 ResponseWriter。
//
// ⚠️ 这是**约定，不是语言特性**。Go 不会自动调用它 ——
// ResponseController 的源码就是一个手写的 for + type switch：
// 「这一层支持吗？不支持就 Unwrap 剥一层，再问。」
// 和 errors.Is 剥 error 链是同一个套路（D2）。
//
// 所以它**不会**让 w.(http.Flusher) 变成 true，只是让愿意配合的调用方能找到底层。
// handler 那边要写：rc := http.NewResponseController(w); rc.Flush()
func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Flush 转发给底层，服务于**直接做类型断言**的调用方。
//
// 为什么 Unwrap 之外还要有它：ResponseController 是 Go 1.20 才有的约定，
// 老代码和第三方库（比如 gorilla/websocket）仍然直接写 w.(http.Flusher) ——
// 它们享受不到 Unwrap。两个都提供才能兼容新老调用方。
//
// ⭐ Flusher 可以安全转发，因为 flush 之后后续的 Write/WriteHeader **仍然经过这里**，
// 统计不会失真。判据是「转发之后我这一层还在链路上吗」。
//
// ⚠️ 对照 Hijacker：**不转发**。文档明说
// 「After a call to Hijack the HTTP server library will not do anything else
// with the connection」—— 连接被接管后 handler 直接写 net.Conn，
// 这里的 status/bytes 再也不会被更新，实测日志会变成 status=0 bytes=0，
// 而客户端实际收到了 418。转发了就是在说谎。
// （顺带：HTTP/2 下压根没有 Hijacker。）
func (r *responseRecorder) Flush() {
	if r.status == 0 {
		r.status = http.StatusOK // flush 会把 header 发出去，等价于隐式 200
	}
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// 编译期断言：确认 Unwrap 的名字和签名没写错。
//
// ⚠️ 这类约定接口写错了是**静默失效** —— 小写的 unwrap、或者返回 any，
// 编译器都不会报错（隐式实现，D5 §1），只是没人认得它。
var _ interface{ Unwrap() http.ResponseWriter } = (*responseRecorder)(nil)

// Logging 记录每个请求的访问日志。
//
// TODO(D11)：实现我。
//
// 要点：
//   - 状态码要**准确**，包括 404 / 500 / 429，也包括 handler 只 Write 没 WriteHeader 的情况
//     （那时是隐式 200，§3）
//   - 耗时要包含内层所有中间件 —— 所以 Logging 通常放最外层
//   - 拿得到 RequestID 就填进去（拿不到留空，不能 panic）
//
// 提示：包装 http.ResponseWriter（§5）。用嵌入，只重写你关心的方法。
//
// ⚠️ 包装会丢掉可选接口（Flusher / Hijacker）—— review 时我会问你怎么处理。
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rr, r)

			// ⭐ 用 InfoContext 而不是 Info —— 普通的 Info 传的是 context.Background()，
			// WithRequestID 那个 Handler 就提取不到 request ID 了。
			//
			// ⭐ 用强类型 attr（slog.String/Int/Float64）而不是 "k", v 键值对：
			// 这是每个请求都要走的热路径，强类型不走反射；而且键值对形式
			// 参数个数写错会静默变成 !BADKEY，编译器不管。
			logger.InfoContext(r.Context(), "请求完成",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rr.status),
				slog.Int("bytes", rr.bytes),

				// 耗时记成【毫秒浮点】，而不是 slog.Duration 或整数毫秒。
				//
				// ⚠️ slog.Duration(d) 在 JSON 里输出的是【纳秒整数】（15ms → 15000000）,
				// 字段名里又看不出单位 —— 日志平台上没人知道那是什么量纲。
				//
				// ⚠️ d.Milliseconds() 更糟：它是 int64 截断除法，实测
				//    45µs  → 0
				//    800µs → 0
				// 亚毫秒请求全被记成 0，算 p50 会得到一堆 0，快慢完全看不出来。
				//
				// 毫秒浮点三者兼顾：45µs → 0.045，1.2s → 1200。
				// 名字里带单位（_ms）、可聚合、亚毫秒不丢。
				//
				// 用 .Nanoseconds() 而不是直接 float64(d)：后者也对（Duration
				// 底层就是 int64 纳秒），但读的人得先想清楚底层单位才敢确认。
				//
				// ⭐ 一般规则（D7）：给【机器】读的日志，时间一律用固定单位的数字，
				// 单位写进字段名。d.String() 那种 "45µs"/"1.2s" 是不同量纲的字符串，
				// 没法排序也没法求和，只适合给人看的 Text 日志。
				slog.Float64("duration_ms", float64(time.Since(start).Nanoseconds())/1e6))
		})
	}
}

// RateLimit 用令牌桶限流，超限返回 429。
//
// TODO(D11)：实现我。
//
// 参数：capacity 是桶容量（也就是允许的突发量），refill 是「多久补一个令牌」。
//
// 行为：
//   - 桶里有令牌就放行并消耗一个
//   - 没有就返回 429，并设置 Retry-After 头（秒数）
//   - **必须并发安全** —— 测试会用 -race 跑几百个并发请求
//
// ⚠️ 想清楚锁的粒度：整个中间件一把锁？还是每个 key 一把？
// 这一版全局一个桶就行（不区分客户端），但 review 时我会问「按 IP 限流该怎么改」。
//
// 提示：不需要真的起定时器。记住「上次补充的时刻」，每次请求时按经过的时间算该补多少。
func RateLimit(capacity int, refill time.Duration) Middleware {
	var mu sync.Mutex
	tokens := float64(capacity)
	lastRefill := time.Now()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			now := time.Now()
			// 根据经过的时间补充令牌
			elapsed := now.Sub(lastRefill)
			added := float64(elapsed) / float64(refill)
			tokens += added
			if tokens > float64(capacity) {
				tokens = float64(capacity)
			}
			lastRefill = now

			if tokens >= 1 {
				tokens -= 1
				mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}

			// 计算还需要等多久才能补 1 个令牌
			retryAfter := time.Duration(float64(refill)).Seconds()
			mu.Unlock()

			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
		})
	}
}
