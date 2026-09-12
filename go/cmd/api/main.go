// Command api 是 D14 的综合项目：把前五天的积木组装成一个能跑的服务。
//
//	cd go
//	make db-migrate
//	go run ./cmd/api
//
//	curl -s localhost:8080/healthz
//	curl -s -XPOST localhost:8080/orders \
//	     -d '{"customer":"alice","items":[{"sku":"A-1","qty":2,"price_cents":500}]}'
//	curl -s localhost:8080/orders/1
//	curl -s -XPOST localhost:8080/orders/1/pay
//
// ⭐ 本文件只做「进程级」的事：读配置、连库、起服务、优雅关闭。
// 【组装】那一步在 internal/server 里 —— 见那个包的注释解释为什么。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // 副作用导入：注册 pgx 驱动（D13 §2）

	"github.com/hzjconan/learn-program-language/go/internal/config"
	"github.com/hzjconan/learn-program-language/go/internal/server"
)

func main() {
	// ⭐ main 只负责退出码，真正的逻辑在 run() 里（§5）。
	//
	// 为什么不直接在 main 里写：
	//   - log.Fatal 会跳过所有 defer —— db.Close() 不会执行
	//   - 返回 error 让【测试可以调用 run】，main 没法测
	//   - 退出码只在一个地方决定
	if err := run(); err != nil {
		//nolint:forbidigo // 启动失败，此刻 logger 可能还没建起来
		println("fatal:", err.Error())
		os.Exit(1)
	}
}

// run 是真正的入口。
//
// TODO(D14)：实现我。按这个顺序：
//
//	① 读配置          config.Load(nil)
//	② 建 logger       slog + config.NewLogger + httpx.WithRequestID
//	                  ⭐ 记一条启动日志，把 cfg 带上（Config.LogValue 保证 token 不泄漏）
//	③ 连数据库        openDB(ctx, cfg)
//	                  设四个池参数（D13 §12）
//	④ 组装三层        orders.NewRepo → ordersvc.New → api.NewRouter
//	⑤ 挂中间件        httpx.Chain（顺序见 D11 §4）
//	⑥ 起服务 + 优雅关闭
//
// ⚠️ 每一步失败都要 return 一个【带上下文】的错误：
// "连接数据库: %w" 比裸的 "connection refused" 好排查得多。
func run() error {
	// ① 读配置
	cfg, err := config.Load(nil)
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}

	// ② 建 logger
	logger := config.NewLogger(cfg, os.Stdout)
	logger.Info("启动服务", "config", cfg)

	// ③ 连数据库 —— 带超时的上下文
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	db, err := openDB(dbCtx, cfg)
	if err != nil {
		return fmt.Errorf("连接数据库: %w", err)
	}
	defer db.Close() //nolint:errcheck // 程序关闭时关闭连接池，出错了也没办法了

	// ④⑤ 组装三层 + 挂中间件
	//
	// ⭐ 组装逻辑在 internal/server 里，不在这个文件 —— 这样集成测试
	// （internal/apitest）验证的是【真正会上线的那份组装】。
	// 留在这里的话，测试只能重写一遍，中间件顺序配错都发现不了。
	h := server.NewHandler(cfg, db, logger)

	// ⑥ 起服务 + 优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return serve(ctx, cfg, h, logger)
}

// openDB 打开连接池并验证连通性。
//
// TODO(D14)：实现我。
//
//   - sql.Open("pgx", cfg.DatabaseURL.Reveal())
//   - ⭐ 必须 PingContext（带超时）—— sql.Open 不建立任何连接（D13 §1）
//   - 设 SetMaxOpenConns / SetMaxIdleConns / SetConnMaxLifetime / SetConnMaxIdleTime
//     ⚠️ MaxIdleConns 要等于 MaxOpenConns，默认值 2 会让多余连接反复建关（D13 §12）
//
// ⚠️ DatabaseURL 里有密码 —— 它在 config 里应该是 Secret 类型（D12 §2），
// 错误信息里别把它打出来。
func openDB(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	// DATABASE_URL 是必须的
	if cfg.DatabaseURL.Reveal() == "" {
		return nil, fmt.Errorf("DATABASE_URL 未配置")
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL.Reveal())
	if err != nil {
		return nil, fmt.Errorf("创建连接池出错: %w", err)
	}

	// D13 §12：四个池参数
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(100) // = MaxOpen，避免多余连接反复建关
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// sql.Open 不建立任何连接，必须 Ping 验证
	if err := db.PingContext(ctx); err != nil {
		db.Close() //nolint:errcheck // 启动失败，关池子
		return nil, fmt.Errorf("验证数据库连通性失败: %w", err)
	}

	return db, nil
}

// serve 起 HTTP 服务并处理优雅关闭。
//
// TODO(D14)：实现我。骨架见讲义 §5，三个细节【一个都不能漏】：
//
//  1. ⚠️ errCh 必须有缓冲（容量 1）
//     Shutdown 之后 ListenAndServe 会返回 ErrServerClosed，
//     没人收的话那条 goroutine 永远阻塞在发送上 ——
//     ⭐ 优雅关闭本身造成了 goroutine 泄漏（D8 §7）
//
//  2. ⚠️ Shutdown 要用【全新的】 context.Background() + 超时，
//     不能用那个已经被信号取消的 ctx —— 否则它立刻超时，等于没等
//
//  3. ⚠️ http.ErrServerClosed 不是错误
//     正常关闭时 ListenAndServe 就返回它，当成失败会让退出码变成 1，
//     K8s 会以为 Pod 崩了
//
// 另外：四个超时都要从 cfg 设上（D11 §8），别只设 Addr 和 Handler。
//
// 用 signal.NotifyContext 监听 os.Interrupt 和 syscall.SIGTERM。
// ⚠️ SIGTERM 是 K8s 停 Pod 时发的信号 —— 只监听 Interrupt 的话，
// 容器里的优雅关闭根本不会触发。
func serve(ctx context.Context, cfg config.Config, h http.Handler, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      h,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1) // 缓冲容量 1 —— D8 §7 防 goroutine 泄漏
	go func() {
		logger.Info("HTTP 服务启动", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("收到关闭信号，开始优雅关闭...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("优雅关闭失败: %w", err)
		}
		logger.Info("HTTP 服务已优雅关闭")
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务异常: %w", err)
	}
}
