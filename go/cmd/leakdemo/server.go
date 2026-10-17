package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// upstreamTimeout 是调行情上游的超时。上游 P50 ≈ 40ms，偶尔很慢。
const upstreamTimeout = 50 * time.Millisecond

type server struct {
	logger *slog.Logger
	seen   *dedup
}

func newServer(logger *slog.Logger) *server {
	return &server{logger: logger, seen: newDedup(50_000)}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /quote", s.quote)
	mux.HandleFunc("POST /ingest", s.ingest)
	return mux
}

// quote 查一个代码的最新价。上游超时就回 504，不拖住调用方。
func (s *server) quote(w http.ResponseWriter, r *http.Request) {
	sym := r.URL.Query().Get("sym")
	if sym == "" {
		http.Error(w, "missing query param sym", http.StatusBadRequest)
		return
	}

	price, err := withTimeout(r.Context(), upstreamTimeout, func() float64 {
		return fetchPrice(sym)
	})
	if errors.Is(err, errTimeout) {
		s.logger.Warn("upstream timeout", "sym", sym, "timeout", upstreamTimeout)
		http.Error(w, fmt.Sprintf("upstream timeout after %s", upstreamTimeout), http.StatusGatewayTimeout)
		return
	}
	if err != nil { // 调用方断开了
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"sym": sym, "price": price}) //nolint:errcheck // 写响应失败 = 客户端断开，没有可做的
}

// fetchPrice 模拟一个不支持 context 的老 SDK：延迟 0–100ms，取消不了。
func fetchPrice(sym string) float64 {
	time.Sleep(time.Duration(rand.IntN(100)) * time.Millisecond)
	return 100 + float64(len(sym))
}

// maxIngestBytes 是单条上报的上限。
const maxIngestBytes = 64 << 10

// 用sync.Pool改写之前的buf := make([]byte, maxIngestBytes)
var bufPool = sync.Pool{
	New: func() any { b := make([]byte, maxIngestBytes); return &b },
}

// ingest 接收客户端上报的一条事件。同一条事件客户端可能重发，按前 16 字节签名去重。
func (s *server) ingest(w http.ResponseWriter, r *http.Request) {
	bufP := bufPool.Get().(*[]byte) //nolint:errcheck // bufPool.New 只返回 *[]byte，没有别的地方往里 Put
	// 放进池子的是*[]byte，如果放切片]byte，因为Put的参数类型是any，需要装箱，又需要额外分配一次 - 复制切片
	// 放回去前不需要清理，下面io.ReadFull是从切片第一位开始“覆盖写”，所以如果第一个请求写入了5个A，第二个请求写入了3个B，
	// 那么buf里面实际的数据是【BBBAA】，下面读buf[:sigLen]，已经保证了sigLen <= n，所以一定不会读入上一次的旧数据 - AAA
	// buf不能放特别大的对象，这里的缓冲区固定 64KB，ReadFull 只覆盖写、不会扩容，所以这个问题不存在
	defer bufPool.Put(bufP)
	buf := *bufP
	n, err := io.ReadFull(io.LimitReader(r.Body, maxIngestBytes), buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if n < sigLen {
		http.Error(w, fmt.Sprintf("body too short: %d bytes, need at least %d", n, sigLen), http.StatusBadRequest)
		return
	}

	if !s.seen.add(buf[:sigLen]) {
		w.WriteHeader(http.StatusOK) // 重复上报，幂等地吞掉
		return
	}
	// ……真正的处理（写队列之类）省略……
	w.WriteHeader(http.StatusAccepted)
}
