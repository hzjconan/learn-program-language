// Command httpdemo 是 lessons/D11.md 的可运行验证。
//
// 全部用 httptest 在内存里跑，不占端口、不用手动访问浏览器。
//
// 核心要建立的两个直觉：
//
//  1. **中间件是「包装」不是「next 回调」** —— 洋葱模型，但没有 next 参数
//
//  2. **http.ListenAndServe 的默认超时是「永不超时」** —— 生产上这是事故
//
//     go run ./cmd/httpdemo
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"
)

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

// ---------- ① 路由：Go 1.22 起标准库够用了 ----------

func demoRouting() {
	title("① ServeMux：方法路由 + 路径参数（1.22+）")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "取 item %s", r.PathValue("id")) //nolint:errcheck // 演示代码；真实 handler 里写响应失败通常只能记日志
	})
	mux.HandleFunc("POST /items/{id}/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "给 %s 打标签 %s", r.PathValue("id"), r.PathValue("tag")) //nolint:errcheck // 演示代码；真实 handler 里写响应失败通常只能记日志
	})
	mux.HandleFunc("GET /files/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "文件路径 %s", r.PathValue("path")) //nolint:errcheck // 演示代码；真实 handler 里写响应失败通常只能记日志
	})

	for _, tc := range [][2]string{
		{"GET", "/items/42"},
		{"POST", "/items/7/tags/hot"},
		{"GET", "/files/a/b/c.txt"},
		{"DELETE", "/items/42"},
		{"GET", "/nope"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), tc[0], tc[1], nil))
		fmt.Printf("  %-6s %-20s → %d  %s\n", tc[0], tc[1], rec.Code,
			strings.TrimSpace(rec.Body.String()))
	}
	fmt.Println("  ⭐ 405 和 404 都是自动的，不用自己写")
}

// ---------- ② ResponseWriter 的三个坑 ----------

func demoResponseWriter() {
	title("② ResponseWriter：写了就定死了")

	// 坑 1：WriteHeader 之后改 header 无效
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusCreated)
	rec.Header().Set("X-Too-Late", "yes") // 太晚了
	fmt.Printf("  WriteHeader 之后设的 header: %q  ← 丢了\n",
		rec.Result().Header.Get("X-Too-Late"))

	// 坑 2：第一次 Write 会隐式 WriteHeader(200)
	rec2 := httptest.NewRecorder()
	rec2.Write([]byte("先写了点数据")) //nolint:errcheck // 演示
	rec2.WriteHeader(http.StatusInternalServerError)
	fmt.Printf("  先 Write 再 WriteHeader(500)，实际状态码: %d  ← 隐式 200 已经定死\n", rec2.Code)

	fmt.Println()
	fmt.Println("  ⭐ 所以错误处理必须在【任何 Write 之前】：")
	fmt.Println("       data, err := compute()")
	fmt.Println("       if err != nil { http.Error(...); return }   ← 先算完再写")
	fmt.Println("       w.Write(data)")
	fmt.Println()
	fmt.Println("  ⚠️ http.Error 会覆盖 Content-Type 为 text/plain —— 想返回 JSON 错误得自己写")
}

// ---------- ③ 中间件：洋葱模型 ----------

// Middleware 的签名固定是这个。
type Middleware func(http.Handler) http.Handler

// Chain 从左到右 = 从外到内。
func Chain(h http.Handler, ms ...Middleware) http.Handler {
	for i := len(ms) - 1; i >= 0; i-- { // ⭐ 倒着包
		h = ms[i](h)
	}
	return h
}

// recorder 记录中间件的进出顺序。
//
// ⚠️ 为什么用 struct 而不是直接传 []string：
// slice header 是【值】，append 会改 len，而 len 只存在于那份拷贝里 ——
// 所以 `func add(log []string, s string) { log = append(log, s) }` 对调用方毫无效果（D3 §7）。
//
// 三种解法：传 *[]string（能用但不地道）、返回新 slice（Go 惯例）、
// 或者像这里一样把状态收进一个 struct、用指针接收者（方法天然能改到字段）。
type recorder struct {
	mu    sync.Mutex
	steps []string
}

func (rec *recorder) add(s string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.steps = append(rec.steps, s)
}

func trace(name string, rec *recorder) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec.add(name + " 前")
			next.ServeHTTP(w, r) // ← 洋葱的核心
			rec.add(name + " 后")
		})
	}
}

func demoMiddleware() {
	title("③ 中间件：包装，不是 next 回调")

	rec := &recorder{}
	h := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec.add("★ handler")
		}),
		trace("A", rec),
		trace("B", rec),
		trace("C", rec),
	)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
	fmt.Printf("  Chain(h, A, B, C) 的执行顺序:\n    %s\n", strings.Join(rec.steps, " → "))
	fmt.Println("  ⭐ 从左到右 = 从外到内，和 Express 的 app.use 一致")
	fmt.Println("     区别：Go 没有 next 参数 —— 「调下一层」就是 next.ServeHTTP(w, r)，")
	fmt.Println("     「不调」就是短路（比如 Auth 失败时直接 return）")
}

// ---------- ④ 包装 ResponseWriter 记录状态码 ----------

type statusRecorder struct {
	http.ResponseWriter // ⭐ 嵌入（D4 §4），自动获得所有方法
	status              int
	bytes               int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK // 隐式 200
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func demoWrapping() {
	title("④ 包装 ResponseWriter 来记录状态码")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello") //nolint:errcheck // 同上
	})
	mux.HandleFunc("GET /bad", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "坏了", http.StatusBadRequest)
	})

	logging := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(rec, r)
			fmt.Printf("  %s %-6s → %d  %d 字节  %v\n",
				r.Method, r.URL.Path, rec.status, rec.bytes,
				time.Since(start).Round(time.Microsecond))
		})
	}

	h := logging(mux)
	for _, p := range []string{"/ok", "/bad", "/none"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), "GET", p, nil))
	}

	fmt.Println()
	fmt.Println("  ⚠️ 包装会【丢掉可选接口】（D5 §5）：真实的 ResponseWriter 可能还实现了")
	fmt.Println("     http.Flusher / http.Hijacker，你的包装类型没有 —— SSE、WebSocket 会失败。")
	fmt.Println("     补救：自己转发一个 Flush()，或者用 Go 1.20+ 的 http.ResponseController。")
}

// ---------- ⑤ panic 会打挂整个进程 ----------

func demoRecover() {
	title("⑤ handler 里 panic：Server 会兜住，但你仍然需要 Recover 中间件")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
		panic("handler 炸了")
	})

	// 不加 Recover：httptest 里会直接 panic 出来
	fmt.Println("  不加 Recover 中间件时：")
	func() {
		defer func() {
			if v := recover(); v != nil {
				fmt.Printf("    panic 传到了调用方: %v\n", v)
			}
		}()
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), "GET", "/panic", nil))
	}()

	recoverMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					// 真实代码这里要打堆栈：debug.Stack()
					http.Error(w, "内部错误", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}

	rec := httptest.NewRecorder()
	recoverMW(mux).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), "GET", "/panic", nil))
	fmt.Printf("  加了 Recover 之后: %d %s", rec.Code, rec.Body.String())

	fmt.Println()
	fmt.Println("  ⚠️ 真实的 http.Server 确实会兜住每个 handler 的 panic（只挂那一个连接），")
	fmt.Println("     但它【不会返回像样的响应】，客户端看到的是连接被重置。")
	fmt.Println("     而且 goroutine 里的 panic 它兜不住 —— 那会打挂整个进程（D8 §1）。")
}

// ---------- ⑥ 四个超时 —— 今天的题眼 ----------

func demoTimeouts() {
	title("⑥ http.Server 的四个超时：默认值是「永不超时」")

	zero := &http.Server{}
	fmt.Println("  零值 http.Server（也就是 http.ListenAndServe 用的那个）：")
	fmt.Printf("    ReadHeaderTimeout = %v\n", zero.ReadHeaderTimeout)
	fmt.Printf("    ReadTimeout       = %v\n", zero.ReadTimeout)
	fmt.Printf("    WriteTimeout      = %v\n", zero.WriteTimeout)
	fmt.Printf("    IdleTimeout       = %v\n", zero.IdleTimeout)
	fmt.Println("  ⚠️ 全是 0 = 永不超时")

	fmt.Println()
	fmt.Println("  后果：")
	fmt.Println("    · Slowloris 攻击：每秒发一个字节的请求头 → 连接永远挂着")
	fmt.Println("    · 客户端连上不发数据 → 一条 goroutine 永远阻塞在读")
	fmt.Println("    · 下游卡住、响应写不出去 → goroutine + 连接一起泄漏")
	fmt.Println()
	fmt.Println("  ⭐ 生产必须显式构造 http.Server：")
	fmt.Println("       srv := &http.Server{")
	fmt.Println("           ReadHeaderTimeout: 5 * time.Second,   // 至少这个")
	fmt.Println("           ReadTimeout:       10 * time.Second,")
	fmt.Println("           WriteTimeout:      15 * time.Second,")
	fmt.Println("           IdleTimeout:       60 * time.Second,")
	fmt.Println("       }")
	fmt.Println("  （golangci-lint 的 gosec G112 专门查 ReadHeaderTimeout）")
}

// ---------- ⑦ 客户端：DefaultClient 没有超时 ----------

func demoClient() {
	title("⑦ 客户端：http.DefaultClient 没有超时")

	fmt.Printf("  http.DefaultClient.Timeout = %v   ← 0 = 永不超时\n", http.DefaultClient.Timeout)
	fmt.Println("  所以 http.Get / http.Post 在生产里一律不要用。")

	// 起一个慢服务
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		fmt.Fprint(w, "慢响应") //nolint:errcheck // 同上
	}))
	defer slow.Close()

	client := &http.Client{Timeout: 50 * time.Millisecond}
	start := time.Now()
	// ⭐ 正确姿势：NewRequestWithContext + Do，而不是 client.Get
	// （linter 的 noctx 规则专门查这个 —— 和 D10 那条「ctx 要一路传下去」同源）
	// ⚠️ 这里【不能】用 log.Fatal —— 它内部调 os.Exit，会跳过所有 defer，
	// 上面那句 defer slow.Close() 就不会执行（D2 §5 那条，gocritic 的 exitAfterDefer 会抓）。
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, slow.URL, nil)
	if err != nil {
		fmt.Println("  构造请求失败:", err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("\n  配了 50ms 超时的 Client: %v 后失败\n", time.Since(start).Round(10*time.Millisecond))
		var ue *url.Error
		if errors.As(err, &ue) {
			fmt.Printf("    是超时吗: %v\n", ue.Timeout())
		}
	} else {
		defer resp.Body.Close()        //nolint:errcheck // demo
		io.Copy(io.Discard, resp.Body) //nolint:errcheck // 演示：读完 body 才能复用连接
	}

	fmt.Println()
	fmt.Println("  四条必须知道的：")
	fmt.Println("    1. DefaultClient 没超时 → 生产别用 http.Get/http.Post")
	fmt.Println("    2. resp.Body 必须 Close，否则连接不还回池子（bodyclose linter 会抓）")
	fmt.Println("    3. body 要【读完】才能复用连接 → io.Copy(io.Discard, resp.Body)")
	fmt.Println("    4. MaxIdleConnsPerHost 默认只有 2 —— 只调一个下游时是经典瓶颈")
}

func main() {
	log.SetFlags(0)
	demoRouting()
	demoResponseWriter()
	demoMiddleware()
	demoWrapping()
	demoRecover()
	demoTimeouts()
	demoClient()
}
