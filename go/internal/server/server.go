// Package server 是本项目的【组装根】（composition root，D14 §2.2）。
//
// # 为什么单独一个包
//
// 它是唯一同时知道所有层的地方：api / ordersvc / orders / httpx 全都 import。
//
// ⚠️ 别把这段逻辑放进 internal/api —— `api` 定义 `OrderService` 接口的目的
// 就是【不依赖】ordersvc。让它为了组装去 import ordersvc，那个接口对 api
// 自己就白定义了。
//
//	层（api / ordersvc / orders）   只知道自己定义的接口，不知道下层是谁
//	组装根（本包）                  必须知道所有层
//
// ⭐ 把组装根塞进任何一层，都会让那一层被迫知道它本不该知道的东西 ——
// 那就是依赖倒置被反转的样子。
//
// # 为什么不留在 cmd/api
//
// 两个理由：
//
//  1. **可测试** —— 集成测试（internal/apitest）要验证的是【真正会上线的
//     那份组装】，不是测试里重写的一份。中间件顺序配错、忘了传 logger、
//     RateLimit 参数写错 —— 只有共用同一个函数才抓得到。
//  2. **可复用** —— 将来加 cmd/grpc 入口时能共用（D14 §2 反复用的那个判据：
//     「加一个 gRPC 入口要改哪些包」）。留在 cmd/api 里就只能复制一遍。
package server

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/hzjconan/learn-program-language/go/internal/api"
	"github.com/hzjconan/learn-program-language/go/internal/config"
	"github.com/hzjconan/learn-program-language/go/internal/httpx"
	"github.com/hzjconan/learn-program-language/go/internal/orders"
	"github.com/hzjconan/learn-program-language/go/internal/ordersvc"
)

// NewHandler 把三层 + 中间件组装成一个 http.Handler。
//
// ⭐ 依赖箭头全部指向内层，一个反向的都没有：
// 本包依赖所有人，没人依赖本包（除了 cmd/api 和集成测试）。
func NewHandler(cfg config.Config, db *sql.DB, logger *slog.Logger) http.Handler {
	// 组装三层。⭐ db 同时满足 api.Pinger（只有一个 PingContext 方法，D14 §2.4）
	repo := orders.NewRepo(db)
	svc := ordersvc.New(repo)
	router := api.NewRouter(svc, db, logger)

	// 挂中间件。顺序见 D11 §4：
	//   RequestID  最外 —— 它用 r.WithContext 造新 request，放内层的话后面拿不到 ID
	//   Recover    靠外 —— 要兜住所有内层的 panic
	//   RateLimit  在业务之前拒绝，省下后面的开销
	//   Logging    要记录包括限流拒绝在内的所有请求
	return httpx.Chain(router,
		httpx.RequestID,
		httpx.Recover(onPanic(logger)),
		httpx.RateLimit(cfg.RateLimit, time.Minute),
		httpx.Logging(logger),
	)
}

// onPanic 把 Recover 中间件兜住的 panic 记进日志。
//
// ⚠️ 用 ErrorContext 而不是 Error —— 这样 WithRequestID 那个 Handler
// 才能把 request_id 注进去（D12 §3），排查时才能把这条 panic 和
// 那次请求的访问日志对上。
func onPanic(logger *slog.Logger) httpx.PanicHandler {
	return func(r *http.Request, recovered any, stack []byte) {
		logger.ErrorContext(r.Context(), "panic recovered",
			"panic", recovered,
			"method", r.Method,
			"path", r.URL.Path,
			"stack", string(stack),
		)
	}
}
