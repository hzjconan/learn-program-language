package main

import (
	"bytes"
	"crypto/rand"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkIngest 是 D17 练习 ④ 的测量工具：一次 /ingest 请求的耗时和分配。
//
// 每轮的前 16 字节（签名）都不同，走的是「新签名、要存进 dedup」那条路径。
// 加 sync.Pool 前后各跑一次，用 benchstat 对比（讲义 §5、§7）。
func BenchmarkIngest(b *testing.B) {
	s := newServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := s.routes()
	body := make([]byte, 4<<10) // 真实上报 4KB 左右，和 load.go 一致
	for b.Loop() {
		_, _ = rand.Read(body[:sigLen])
		req := httptest.NewRequestWithContext(b.Context(), http.MethodPost, "/ingest", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			b.Fatalf("status = %d, want 202", rec.Code)
		}
	}
}
