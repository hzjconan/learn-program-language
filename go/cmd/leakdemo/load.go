package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// runLoad 往 addr 打 n 个请求（quote 和 ingest 各一半），conc 个并发。
// 它只是压测工具，不是靶子的一部分 —— 可以放心读。
func runLoad(addr string, n, conc int) error {
	base := "http://localhost" + addr
	if addr != "" && addr[0] != ':' {
		base = "http://" + addr
	}
	client := &http.Client{Timeout: 5 * time.Second}

	var (
		next   atomic.Int64
		counts sync.Map // status code → *atomic.Int64
		wg     sync.WaitGroup
	)
	count := func(code int) {
		v, _ := counts.LoadOrStore(code, new(atomic.Int64))
		v.(*atomic.Int64).Add(1) //nolint:errcheck // LoadOrStore 只会存 *atomic.Int64
	}

	start := time.Now()
	for range conc {
		wg.Go(func() {
			for i := next.Add(1); i <= int64(n); i = next.Add(1) {
				code, err := hit(client, base, i)
				if err != nil {
					code = -1
				}
				count(code)
			}
		})
	}
	wg.Wait()

	fmt.Printf("%d 个请求，用时 %s\n", n, time.Since(start).Round(time.Millisecond))
	counts.Range(func(k, v any) bool {
		fmt.Printf("  status %v: %d\n", k, v.(*atomic.Int64).Load()) //nolint:errcheck // 同上
		return true
	})
	return nil
}

func hit(client *http.Client, base string, i int64) (int, error) {
	ctx := context.Background()
	var req *http.Request
	var err error
	if i%2 == 0 {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/quote?sym=S%d", base, i%500), nil)
	} else {
		body := make([]byte, 4<<10) // 真实上报 4KB 左右
		_, _ = rand.Read(body)
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/ingest", bytes.NewReader(body))
	}
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()               //nolint:errcheck // 读完就关
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // 读完 body 才能复用连接；压测工具不关心失败
	return resp.StatusCode, nil
}
