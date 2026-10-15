// Command leakdemo 是 D16 的 pprof 靶子：一个「看起来挺正常」的行情小服务。
//
// ⚠️ 它有两处问题：一处 goroutine 泄漏，一处内存泄漏。
// ⚠️ 规则：先【别读】server.go / helpers.go。只靠 pprof 定位到文件和行号，再回来读代码。
// 这正是线上的处境 —— 告警响的时候，你面对的是几十万行别人写的代码，
// 能指路的只有 profile（讲义 §6）。
//
// 终端 1：起服务
//
//	go run ./cmd/leakdemo
//
// 终端 2：压一轮（默认 3000 个请求、20 并发）
//
//	go run ./cmd/leakdemo -load
//
// 然后看 http://localhost:6060/debug/pprof/ 。练习要求见 lessons/D16.md §8 ⑤。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var (
		addr      = flag.String("addr", ":8095", "业务端口")
		adminAddr = flag.String("admin-addr", "localhost:6060", "pprof 端口（⚠️ 只监听 localhost，§5.2）")
		load      = flag.Bool("load", false, "压测模式：往 -addr 打请求，然后退出")
		n         = flag.Int("n", 3000, "压测请求数")
		conc      = flag.Int("c", 20, "压测并发")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *load {
		if err := runLoad(*addr, *n, *conc); err != nil {
			logger.Error("压测失败", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := run(*addr, *adminAddr, logger); err != nil {
		logger.Error("服务退出", "err", err)
		os.Exit(1)
	}
}

func run(addr, adminAddr string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := &http.Server{
		Addr:              addr,
		Handler:           newServer(logger).routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	// ⭐ pprof 走独立的 mux + 独立的端口（§5.2）。
	// 业务 mux 上【没有】 /debug/pprof —— 公网流量摸不到。
	admin := &http.Server{
		Addr:              adminAddr,
		Handler:           adminMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	for _, srv := range []*http.Server{app, admin} {
		go func() {
			logger.Info("listening", "addr", srv.Addr)
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("listen %s: %w", srv.Addr, err)
			}
		}()
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return errors.Join(app.Shutdown(shutdownCtx), admin.Shutdown(shutdownCtx))
}

// adminMux 显式挂 pprof 的五个 handler，而不是用 http.DefaultServeMux。
//
// ⚠️ import "net/http/pprof" 的副作用是往 DefaultServeMux 注册 /debug/pprof/*。
// 如果你的业务服务用的是 DefaultServeMux（http.ListenAndServe(addr, nil)），
// pprof 就跟着业务一起暴露到公网了（§5.2）。
func adminMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index) // heap、goroutine、allocs… 都由 Index 按名字分发
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}
