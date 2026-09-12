// Package api 是 HTTP 传输层（D14）。
//
// # 它负责什么
//
//	解析 HTTP     路径参数、查询参数、请求体 → 领域类型
//	写 HTTP       领域类型 → JSON；apperr.Kind → 状态码
//
// # 它【不】负责什么
//
//	⚠️ 不做业务校验 —— 那是 ordersvc 的事（§3）
//	⚠️ 不碰 SQL
//
// ⭐ 和 ordersvc 一样，本包定义它【需要】的接口（OrderService / Pinger），
// 而不是 import 具体实现。所以它的测试用一个假 service 就能跑，
// 不需要数据库，也不需要真的业务逻辑。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
	"github.com/hzjconan/learn-program-language/go/internal/orders"
)

// OrderService 是 api 层需要的业务能力。
//
// ⭐ 定义在消费方（§2.2）。ordersvc.Service 恰好满足它，但它自己不知道。
type OrderService interface {
	Place(ctx context.Context, o *orders.Order) error
	Get(ctx context.Context, id int64) (*orders.Order, error)
	List(ctx context.Context, f orders.ListFilter) ([]orders.Order, error)
	Pay(ctx context.Context, id int64) error
	Cancel(ctx context.Context, id int64) error
}

// Pinger 是健康检查需要的能力。
//
// ⭐ 只有一个方法 —— 这就是「按需要切接口」（§2.4）。
// *sql.DB 恰好满足它，测试里传个假的就行。
type Pinger interface {
	PingContext(ctx context.Context) error
}

// MaxBodyBytes 是请求体大小上限（D11 §6）。
const MaxBodyBytes = 1 << 20 // 1MB

// HealthCheckTimeout 是健康检查里 Ping 数据库的超时。
//
// ⚠️ 必须设 —— 否则数据库挂住时健康检查也会挂住，
// K8s 的 liveness probe 就永远等不到回复，只能靠它自己的超时。
const HealthCheckTimeout = 2 * time.Second

// ---------- DTO ----------
//
// ⭐ 这些类型都是【小写】的：它们是本包的实现细节，不该被别的包引用。
// API 契约由 JSON 定义，不由 Go 类型定义（§4）。
//
// ⚠️ 绝对不要直接把 orders.Order 当请求体 —— 那样客户端能设置
// ID / Status / TotalCents / CreatedAt，D13 那条「金额不信调用方」就白做了。

// createOrderRequest 是 POST /orders 的请求体。
//
// TODO(D14)：补上字段。
//
// 只该有这三样：customer、note（可选）、items。
// ⚠️ 【不要】有 id / status / total_cents / created_at。
type createOrderRequest struct {
	Customer string            `json:"customer"`
	Note     string            `json:"note"`
	Items    []createItemInput `json:"items"`
}

// createItemInput 是请求体里的一条商品。
//
// TODO(D14)：补上字段（sku / qty / price_cents）。
type createItemInput struct {
	Sku        string `json:"sku"`
	Qty        int    `json:"qty"`
	PriceCents int64  `json:"price_cents"`
}

// orderResponse 是订单的响应体。
//
// TODO(D14)：补上字段。
//
// id / customer / note / status / total_cents / created_at / items。
// ⚠️ note 为空时用 omitempty 省掉（D12 §1.4），别给客户端一个空字符串。
type orderResponse struct {
	ID         int64          `json:"id"`
	Customer   string         `json:"customer"`
	Note       string         `json:"note,omitempty"`
	Status     string         `json:"status"`
	TotalCents int64          `json:"total_cents"`
	CreatedAt  time.Time      `json:"created_at"`
	Items      []itemResponse `json:"items"`
}

// itemResponse 是响应体里的一条商品。
//
// TODO(D14)：补上字段（id / sku / qty / price_cents）。
type itemResponse struct {
	ID         int64  `json:"id"`
	Sku        string `json:"sku"`
	Qty        int    `json:"qty"`
	PriceCents int64  `json:"price_cents"`
}

// toResponse 把领域对象转成响应 DTO。
//
// TODO(D14)：实现我。
//
// ⚠️ Items 为空时要返回 `[]` 而不是 `null`（D12 §1.4 那条：
// 切片的零值是 nil，序列化成 null，前端最烦这个）。
func toResponse(o *orders.Order) orderResponse {
	order := orderResponse{
		ID:         o.ID,
		Customer:   o.Customer,
		Note:       o.Note,
		Status:     o.Status,
		TotalCents: o.TotalCents,
		CreatedAt:  o.CreatedAt,
		Items:      make([]itemResponse, 0, len(o.Items)),
	}
	for _, item := range o.Items {
		order.Items = append(order.Items, itemResponse{
			ID:         item.ID,
			Sku:        item.SKU,
			Qty:        item.Qty,
			PriceCents: item.PriceCents,
		})
	}
	return order
}

// toDomain 把请求 DTO 转成领域对象。
//
// TODO(D14)：实现我。
//
// ⭐ 只搬字段，【不做业务校验】—— 校验是 ordersvc 的职责（§3）。
// 这里唯一该做的是「结构转换」。
func (req createOrderRequest) toDomain() *orders.Order {
	order := &orders.Order{
		Customer: req.Customer,
		Note:     req.Note,
		Items:    make([]orders.Item, 0, len(req.Items)),
	}
	for _, item := range req.Items {
		order.Items = append(order.Items, orders.Item{
			SKU:        item.Sku,
			Qty:        item.Qty,
			PriceCents: item.PriceCents,
		})
	}
	return order
}

// ---------- 路由 ----------

// NewRouter 组装所有 HTTP 端点。
//
// TODO(D14)：实现我。
//
//	POST   /orders               → 201，响应体是 orderResponse
//	GET    /orders/{id}          → 200
//	GET    /orders               → 200，响应体是 {"orders":[...]}
//	POST   /orders/{id}/pay      → 200，返回更新后的订单
//	POST   /orders/{id}/cancel   → 200，返回更新后的订单
//	GET    /healthz              → 200 或 503
//
// 用 Go 1.22 的路由语法（D11 §2），405 会自动处理。
//
// ⚠️ 五个硬要求：
//
//  1. 请求体用 http.MaxBytesReader 限制 MaxBodyBytes，超了返回 413
//  2. 解码用 DisallowUnknownFields（D12 §1.3）—— 客户端拼错字段名要 400，不能静默忽略
//  3. 路径参数解析失败（/orders/abc）→ 400，不要送到 service 去
//  4. 所有错误响应统一走 writeError
//  5. ⚠️ 别在这里写业务校验 —— 「至少一个 item」这种规则属于 ordersvc
func NewRouter(svc OrderService, health Pinger, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// POST /orders
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		limited := http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		defer limited.Close() //nolint:errcheck // 请求处理结束后关闭reader，出错了也没办法了

		var req createOrderRequest
		dec := json.NewDecoder(limited)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			if isMaxBytesError(err) {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
					"error": "请求体过大",
				})
				return
			}
			writeError(w, r, logger, apperr.Invalid("请求体格式不合法", nil))
			return
		}

		o := req.toDomain()
		if err := svc.Place(r.Context(), o); err != nil {
			writeError(w, r, logger, err)
			return
		}

		writeJSON(w, http.StatusCreated, toResponse(o))
	})

	// GET /orders/{id}
	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		o, err := svc.Get(r.Context(), id)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toResponse(o))
	})

	// GET /orders
	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		f, err := parseListFilter(r)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		list, err := svc.List(r.Context(), f)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		resp := make([]orderResponse, 0, len(list))
		for i := range list {
			resp = append(resp, toResponse(&list[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"orders": resp})
	})

	// POST /orders/{id}/pay
	mux.HandleFunc("POST /orders/{id}/pay", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		if err = svc.Pay(r.Context(), id); err != nil {
			writeError(w, r, logger, err)
			return
		}
		o, err := svc.Get(r.Context(), id)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toResponse(o))
	})

	// POST /orders/{id}/cancel
	mux.HandleFunc("POST /orders/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		if err := svc.Cancel(r.Context(), id); err != nil {
			writeError(w, r, logger, err)
			return
		}
		o, err := svc.Get(r.Context(), id)
		if err != nil {
			writeError(w, r, logger, err)
			return
		}
		writeJSON(w, http.StatusOK, toResponse(o))
	})

	// GET /healthz
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), HealthCheckTimeout)
		defer cancel()
		if err := health.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return mux
}

// isMaxBytesError 判断 err 是否是 MaxBytesReader 触发的大小超限。
func isMaxBytesError(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// ---------- 响应辅助 ----------

// writeJSON 写一个 JSON 响应。
//
// TODO(D14)：实现我。
//
// 设 Content-Type、写状态码、编码 body。
// ⚠️ body 为 nil 时不要写 body（204 那种情况）。
// ⚠️ 别用 http.Error —— 它会把 Content-Type 覆盖成 text/plain（D11 §3）。
func writeJSON(w http.ResponseWriter, status int, v any) {
	if v == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) //nolint:errcheck // 响应已开始写、失败无法补救
}

// writeError 是【唯一】的错误出口。
//
// TODO(D14)：实现我。
//
//   - 用 apperr.HTTPStatus(err) 拿到状态码和【用户可见】的消息（D12 §5）
//   - 响应体是 {"error": "消息"}
//   - ⭐ status >= 500 时用 logger.ErrorContext 记完整的 err（含整条 %w 链）；
//     4xx 【不记】error 级别 —— 那是客户端的问题，记了只会淹没真正的故障
//   - ⚠️ 给客户端的消息只能是 apperr 的 Message，绝不能是 err.Error()
//
// ⭐ 用 ErrorContext 而不是 Error —— 这样 WithRequestID 那个 Handler
// 才能把 request_id 注进去（D12 §3）。
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, msg := apperr.HTTPStatus(err)
	writeJSON(w, status, map[string]string{
		"error": msg,
	})
	if status >= 500 {
		logger.ErrorContext(r.Context(), "internal error", "err", err)
	}
}

// parseID 从路径参数里解析订单 id。
//
// TODO(D14)：实现我。
//
// 解析失败 → apperr.Invalid（消息里带上那个非法的值，方便排查）。
// ⚠️ 别返回 0 和 nil 让调用方自己判断 —— 那样每个 handler 都要写一遍检查。
func parseID(r *http.Request) (int64, error) {
	v := r.PathValue("id")
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, apperr.Invalid(fmt.Sprintf("路径参数 id=%q 不合法", v), nil)
	}
	return id, nil
}

// parseListFilter 从查询参数解析过滤条件。
//
// TODO(D14)：实现我。
//
//	?customer=alice   → Customer 指向 "alice"
//	?status=paid      → Status 指向 "paid"
//	?limit=20         → Limit = 20
//
// ⭐ 关键：**参数没出现**和**参数是空字符串**要区分开（D12 §1.2 那套）：
//
//	/orders              → Customer 是 nil（不过滤）
//	/orders?customer=    → Customer 指向 ""（过滤「客户名为空」的订单）
//
// 用 r.URL.Query().Has(key) 判断有没有出现，不要用 Get(key) != ""。
//
// limit 解析失败（?limit=abc）→ apperr.Invalid。
// ⚠️ 别静默当成 0 —— 那样用户会拿到默认条数，还以为自己的参数生效了。
func parseListFilter(r *http.Request) (orders.ListFilter, error) {
	q := r.URL.Query()
	var f orders.ListFilter

	if q.Has("customer") {
		v := q.Get("customer")
		f.Customer = &v
	}
	if q.Has("status") {
		v := q.Get("status")
		f.Status = &v
	}
	if q.Has("limit") {
		v := q.Get("limit")
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, apperr.Invalid(fmt.Sprintf("limit %q 不合法，必须是非负整数", v), nil)
		}
		f.Limit = n
	}

	return f, nil
}

// ---------- 骨架期占位 ----------
//
// 这几行让 unused 这个 linter 知道上面那些类型和函数是【要用的】。
// ⚠️ NewRouter 实现完之后，把这整段删掉 —— 那时它们自然都被引用了。
var (
	_ = createOrderRequest{}
	_ = createItemInput{}
	_ = orderResponse{}
	_ = itemResponse{}
	_ = toResponse
	_ = createOrderRequest{}.toDomain
	_ = writeJSON
	_ = writeError
	_ = parseID
	_ = parseListFilter
)
