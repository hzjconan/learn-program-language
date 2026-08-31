package httpx

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ⚠️ 这个文件是 D11 §7 「WriteTimeout 陷阱」的示例，不是练习。
//
// 它演示一个 SSE（Server-Sent Events）端点：有人往 Store 里写/改/删时，
// 通过一条长连接实时推送给客户端。
//
// 之所以用 SSE 举例，是因为它同时踩中今天和前几天的四个坑：
//
//	D11 §7  WriteTimeout 从【读完请求头】开始算 —— 长连接必然撞上
//	D11 §5  包装 ResponseWriter 会丢 Flusher —— 不 flush 就不产生 chunk
//	D10 §5  客户端断开时 r.Context() 会取消 —— 不检查就是 goroutine 泄漏
//	D8/D9   订阅者管理需要并发安全，且【慢订阅者不能阻塞写入方】

// Event 是一次 Store 变更。
type Event struct {
	// Op 是操作类型："put" 或 "delete"。
	Op string `json:"op"`
	// Key 是被改动的键。
	Key string `json:"key"`
	// Value 是新值；delete 时为空。
	Value string `json:"value,omitempty"`
}

// subscribers 管理所有订阅者。
//
// 单独抽出来而不是塞进 Store，是为了让两把锁的职责清楚：
// Store.mu 保护数据，subs.mu 保护订阅者列表。
// 混在一把锁里的话，广播时会长时间占着数据锁（见 broadcast 的注释）。
type subscribers struct {
	mu   sync.Mutex
	next int
	m    map[int]chan Event
}

// Subscribe 注册一个订阅者，返回事件流和取消订阅的函数。
//
// ⭐ 返回「取消函数」而不是「取消方法」，是 context.WithCancel 的同款设计：
// 调用方拿到什么就能取消什么，不需要额外记一个 id。
func (s *Store) Subscribe() (<-chan Event, func()) {
	s.subs.mu.Lock()
	defer s.subs.mu.Unlock()

	if s.subs.m == nil {
		s.subs.m = make(map[int]chan Event)
	}
	id := s.subs.next
	s.subs.next++

	// ⭐ 带缓冲。缓冲区的大小是个取舍：
	//   太小 → 稍微慢一点的客户端就开始丢事件
	//   太大 → 每个订阅者占的内存变多，而且丢的时候一次丢一大批
	ch := make(chan Event, 16)
	s.subs.m[id] = ch

	var once sync.Once
	return ch, func() {
		// ⭐ 用 sync.Once 保证幂等 —— 调用方很可能 defer 一次、出错路径再调一次。
		// 重复 close 会 panic（D8 §3）。
		once.Do(func() {
			s.subs.mu.Lock()
			defer s.subs.mu.Unlock()
			if c, ok := s.subs.m[id]; ok {
				delete(s.subs.m, id)
				close(c) // 让订阅者的 for range 自然退出
			}
		})
	}
}

// broadcast 把事件推给所有订阅者。
//
// ⚠️⚠️ 两条硬规则，违反任何一条都会拖垮整个服务：
//
//  1. **绝对不能在持有 Store.mu 时调用它。** 广播要拿 subs.mu，
//     而订阅者可能正好在调 Store.Get（要拿 Store.mu）—— 两把锁顺序相反就是
//     经典的 AB-BA 死锁（D9 §11）。所以 Put/Delete 里是【解锁之后】才广播。
//
//  2. **发送必须非阻塞。** 一个卡住的 SSE 客户端（比如手机切后台）会让它的
//     channel 填满，如果这里用阻塞发送，**整个 Store 的写入就全卡死了** ——
//     一个慢客户端拖垮所有人。所以用 select + default 直接丢弃。
//
// 「丢事件」听起来很糟，但它是**有意的取舍**：宁可让慢客户端丢几条，
// 也不能让它拖住写入方。真要保证不丢，就得给每个订阅者一个持久队列（那是另一个系统了）。
func (s *Store) broadcast(e Event) {
	s.subs.mu.Lock()
	defer s.subs.mu.Unlock()

	for _, ch := range s.subs.m {
		select {
		case ch <- e:
		default: // ⭐ 满了就丢，绝不阻塞
		}
	}
}

// EventsHandler 返回一个 SSE handler，把 Store 的变更实时推给客户端。
//
// 用法：mux.Handle("GET /events", EventsHandler(store))
//
//	curl -N http://localhost:8080/events
//	（-N 关闭 curl 的输出缓冲，否则你看不到实时效果）
func EventsHandler(s *Store) http.HandlerFunc {
	// heartbeat 的意义：中间的反向代理通常会掐掉「空闲太久」的连接。
	// 定期发一行注释（SSE 里以 : 开头的行会被客户端忽略）来保活。
	const heartbeat = 15 * time.Second

	// 每次写之前把 deadline 推到这么远。它必须【大于】heartbeat，
	// 否则心跳还没来得及发，连接就被 WriteTimeout 砍了。
	const writeWindow = 30 * time.Second

	return func(w http.ResponseWriter, r *http.Request) {
		// SSE 的三个必需响应头
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// ⭐ D11 §5：用 ResponseController 而不是 w.(http.Flusher)。
		// 中间件（比如 Logging）包装过 w 之后，直接类型断言会失败；
		// ResponseController 会顺着 Unwrap 链找到底层的 Flusher。
		rc := http.NewResponseController(w)

		events, unsubscribe := s.Subscribe()
		defer unsubscribe() // ⭐ 不取消订阅 = channel 永远留在 map 里 = 内存泄漏

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()

		// ⭐ D10 §6：r.Context() 在客户端断开连接时会被取消。
		// 没有这个分支的话，客户端关掉浏览器之后这条 goroutine 会永远跑下去。
		ctx := r.Context()

		// 先发一个空注释，让客户端立刻知道连接建立了
		if err := writeAndFlush(w, rc, ": connected\n\n", writeWindow); err != nil {
			return
		}

		for {
			select {
			case <-ctx.Done():
				// 客户端断开 / 服务优雅关闭
				return

			case e, ok := <-events:
				if !ok {
					return // Store 关掉了这个订阅
				}
				// SSE 的格式：event: <类型>\ndata: <一行 JSON>\n\n
				// data 必须是单行 —— JSON 里的换行已经被 Marshal 转义了，所以安全。
				payload := fmt.Sprintf("event: %s\ndata: {\"key\":%q,\"value\":%q}\n\n",
					e.Op, e.Key, e.Value)
				if err := writeAndFlush(w, rc, payload, writeWindow); err != nil {
					return // 写失败通常意味着客户端已经断了
				}

			case <-ticker.C:
				if err := writeAndFlush(w, rc, ": ping\n\n", writeWindow); err != nil {
					return
				}
			}
		}
	}
}

// writeAndFlush 是这个示例的核心：**每次写之前把写超时往后推**。
//
// ⭐ 为什么必须这么做（D11 §7 那个陷阱）：
//
//	http.Server.WriteTimeout 是从【读完请求头】开始算的，不是从「开始写响应」算。
//	所以一个设了 WriteTimeout: 15s 的服务，任何 SSE 连接都会在 15 秒后被砍断 ——
//	不管你推得多勤快。
//
// 实测（全局 WriteTimeout = 200ms）：
//
//	/sse         ⚠️ 只收到 45/75 字节，中途断
//	/sse-fixed   ✅ 完整收到 75 字节        ← 就是靠这个 SetWriteDeadline
//
// ⭐ 这是比「把全局 WriteTimeout 调大」好得多的做法：
// **全局设紧防 Slowloris，个别长连接自己放宽。**
func writeAndFlush(w http.ResponseWriter, rc *http.ResponseController, s string, window time.Duration) error {
	// 把这一次写的 deadline 推到 window 之后
	if err := rc.SetWriteDeadline(time.Now().Add(window)); err != nil {
		// ⚠️ 不是致命错误：某些 ResponseWriter 实现（比如 httptest.ResponseRecorder）
		// 不支持设置 deadline，返回 http.ErrNotSupported。这时候继续写就行 ——
		// 只是失去了「单独放宽超时」的能力。
		_ = err
	}
	if _, err := fmt.Fprint(w, s); err != nil {
		return err
	}
	return rc.Flush()
}
