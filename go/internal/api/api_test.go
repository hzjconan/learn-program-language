package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hzjconan/learn-program-language/go/internal/api"
	"github.com/hzjconan/learn-program-language/go/internal/apperr"
	"github.com/hzjconan/learn-program-language/go/internal/orders"
)

// ---------- 假 service ----------
//
// ⭐ 又是一个普通 struct（§6）。api 层的测试不需要真的业务逻辑，
// 更不需要数据库 —— 它只该验「HTTP 契约」。
type fakeSvc struct {
	placed   []*orders.Order
	gotID    int64
	gotAct   string // "pay" / "cancel"
	gotFiler orders.ListFilter

	order    *orders.Order
	list     []orders.Order
	err      error // 让所有方法返回这个错误
	assignID int64
}

func (f *fakeSvc) Place(_ context.Context, o *orders.Order) error {
	if f.err != nil {
		return f.err
	}
	if f.assignID != 0 {
		o.ID = f.assignID
	}
	o.Status = orders.StatusPending
	for _, it := range o.Items {
		o.TotalCents += int64(it.Qty) * it.PriceCents
	}
	cp := *o
	f.placed = append(f.placed, &cp)
	return nil
}

func (f *fakeSvc) Get(_ context.Context, id int64) (*orders.Order, error) {
	f.gotID = id
	if f.err != nil {
		return nil, f.err
	}
	return f.order, nil
}

func (f *fakeSvc) List(_ context.Context, fl orders.ListFilter) ([]orders.Order, error) {
	f.gotFiler = fl
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func (f *fakeSvc) Pay(_ context.Context, id int64) error {
	f.gotID, f.gotAct = id, "pay"
	return f.err
}

func (f *fakeSvc) Cancel(_ context.Context, id int64) error {
	f.gotID, f.gotAct = id, "cancel"
	return f.err
}

// fakePinger 是健康检查用的假实现。
type fakePinger struct{ err error }

func (p fakePinger) PingContext(context.Context) error { return p.err }

// ---------- 辅助 ----------

func newTestRouter(svc api.OrderService, p api.Pinger) (http.Handler, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	return api.NewRouter(svc, p, logger), &buf
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// wantJSON 检查响应是 JSON 并解析。
func wantJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json\n"+
			"（⚠️ 用了 http.Error 吗？它会覆盖成 text/plain —— D11 §3）", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return m
}

const validBody = `{"customer":"alice","items":[{"sku":"A-1","qty":2,"price_cents":500}]}`

// ---------- POST /orders ----------

func TestAPI_CreateOrder(t *testing.T) {
	svc := &fakeSvc{assignID: 7}
	h, _ := newTestRouter(svc, fakePinger{})

	rec := do(t, h, "POST", "/orders", validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d, want 201\nbody: %s", rec.Code, rec.Body.String())
	}
	m := wantJSON(t, rec)

	if m["id"] != float64(7) {
		t.Errorf("id = %v, want 7", m["id"])
	}
	if m["customer"] != "alice" {
		t.Errorf("customer = %v", m["customer"])
	}
	if m["status"] != orders.StatusPending {
		t.Errorf("status = %v, want pending", m["status"])
	}
	if m["total_cents"] != float64(1000) {
		t.Errorf("total_cents = %v, want 1000", m["total_cents"])
	}

	if len(svc.placed) != 1 {
		t.Fatalf("service.Place 被调用了 %d 次, want 1", len(svc.placed))
	}
	got := svc.placed[0]
	if got.Customer != "alice" || len(got.Items) != 1 || got.Items[0].SKU != "A-1" {
		t.Errorf("传给 service 的对象不对: %+v", got)
	}
}

// TestAPI_CreateIgnoresClientControlledFields 是 DTO 那一节的核心测试（§4）。
//
// ⚠️ 如果直接把 orders.Order 当请求体，客户端就能设置
// id / status / total_cents / created_at —— D13 那条「金额不信调用方」
// 会被从 HTTP 层直接绕过去。
//
// 正确的 DTO 里根本没有这些字段，所以配合 DisallowUnknownFields，
// 客户端传了会直接 400。
func TestAPI_CreateIgnoresClientControlledFields(t *testing.T) {
	tests := []struct{ name, body string }{
		{"想自己定 id", `{"id":999,"customer":"a","items":[{"sku":"S","qty":1,"price_cents":100}]}`},
		{"想自己定 status", `{"status":"paid","customer":"a","items":[{"sku":"S","qty":1,"price_cents":100}]}`},
		{"想自己定 total_cents", `{"total_cents":1,"customer":"a","items":[{"sku":"S","qty":1,"price_cents":100}]}`},
		{"想自己定 created_at", `{"created_at":"2020-01-01T00:00:00Z","customer":"a","items":[{"sku":"S","qty":1,"price_cents":100}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeSvc{assignID: 1}
			h, _ := newTestRouter(svc, fakePinger{})

			rec := do(t, h, "POST", "/orders", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("状态码 = %d, want 400\n"+
					"（⚠️ DTO 里不该有这个字段，且解码要开 DisallowUnknownFields）\nbody: %s",
					rec.Code, rec.Body.String())
			}
			if len(svc.placed) != 0 {
				t.Errorf("请求被拒绝了却还是调用了 service")
			}
		})
	}
}

func TestAPI_CreateBadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"空 body", "", http.StatusBadRequest},
		{"不是 JSON", `not json`, http.StatusBadRequest},
		{"JSON 但类型不对", `{"customer":123}`, http.StatusBadRequest},
		{"未知字段", `{"customer":"a","itemz":[]}`, http.StatusBadRequest},
		{"超大 body", `{"customer":"` + strings.Repeat("x", 2<<20) + `"}`, http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeSvc{}
			h, _ := newTestRouter(svc, fakePinger{})

			rec := do(t, h, "POST", "/orders", tt.body)
			if rec.Code != tt.want {
				t.Errorf("状态码 = %d, want %d\nbody: %s", rec.Code, tt.want, rec.Body.String())
			}
			wantJSON(t, rec) // 错误响应也必须是 JSON
			if len(svc.placed) != 0 {
				t.Error("请求非法却调用了 service")
			}
		})
	}
}

// TestAPI_CreateDoesNotValidateBusinessRules 锁住分层边界（§3）。
//
// ⭐ 「至少一个 item」是【业务规则】，属于 ordersvc。
// api 层看到一个结构合法的请求，就该原样交给 service ——
// 由 service 决定它合不合业务规则。
//
// ⚠️ 如果 api 层自己也校验一遍，规则就有了两处实现，
// 迟早不一致；而且 gRPC 入口会绕过 api 层的那一份。
func TestAPI_CreateDoesNotValidateBusinessRules(t *testing.T) {
	svc := &fakeSvc{assignID: 1}
	h, _ := newTestRouter(svc, fakePinger{})

	// 结构合法但业务上非法（没有 item）—— 假 service 不报错，所以应该成功
	rec := do(t, h, "POST", "/orders", `{"customer":"alice","items":[]}`)

	if rec.Code == http.StatusBadRequest && len(svc.placed) == 0 {
		t.Error("api 层自己做了业务校验 —— 「至少一个 item」属于 ordersvc（§3）\n" +
			"api 只该管「结构对不对」，不该管「业务允不允许」")
	}
	if len(svc.placed) != 1 {
		t.Errorf("service 被调用了 %d 次, want 1", len(svc.placed))
	}
}

// TestAPI_CreatePropagatesServiceError 确认 service 的 Kind 被正确映射。
func TestAPI_CreatePropagatesServiceError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"Invalid → 400", apperr.Invalid("至少要有一个商品", nil), http.StatusBadRequest},
		{"Conflict → 409", apperr.Conflict("SKU 重复", nil), http.StatusConflict},
		{"Internal → 500", apperr.Internal("写库失败", errors.New("boom")), http.StatusInternalServerError},
		{"普通 error → 500", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestRouter(&fakeSvc{err: tt.err}, fakePinger{})

			rec := do(t, h, "POST", "/orders", validBody)
			if rec.Code != tt.want {
				t.Errorf("状态码 = %d, want %d", rec.Code, tt.want)
			}
			m := wantJSON(t, rec)
			if m["error"] == nil || m["error"] == "" {
				t.Errorf("错误响应里应该有非空的 error 字段，得到 %v", m)
			}
		})
	}
}

// TestAPI_ErrorDoesNotLeakInternals 锁住「内部细节不给用户」（D12 §5）。
func TestAPI_ErrorDoesNotLeakInternals(t *testing.T) {
	const secret = "dial tcp 10.0.0.5:5432: connection refused"
	h, logBuf := newTestRouter(
		&fakeSvc{err: apperr.Internal("写库失败", errors.New(secret))}, fakePinger{})

	rec := do(t, h, "POST", "/orders", validBody)
	m := wantJSON(t, rec)

	msg, _ := m["error"].(string)
	for _, frag := range []string{secret, "10.0.0.5", "5432"} {
		if strings.Contains(msg, frag) {
			t.Errorf("给客户端的消息泄漏了 %q:\n  %s\n"+
				"（writeError 要用 apperr.HTTPStatus 拿 Message，不是 err.Error()）", frag, msg)
		}
	}

	// ⭐ 但日志里必须有完整细节 —— 否则线上什么都查不到
	if !strings.Contains(logBuf.String(), secret) {
		t.Errorf("5xx 的完整错误链应该进日志，日志里没找到 %q:\n%s", secret, logBuf.String())
	}
}

// TestAPI_ClientErrorsAreNotLoggedAsErrors 是可观测性的一条。
//
// ⚠️ 4xx 是客户端的问题。把它们记成 ERROR 级别，会让真正的故障
// 淹没在噪音里 —— 告警规则基于「错误日志速率」时尤其致命。
func TestAPI_ClientErrorsAreNotLoggedAsErrors(t *testing.T) {
	h, logBuf := newTestRouter(&fakeSvc{err: apperr.Invalid("参数不对", nil)}, fakePinger{})

	rec := do(t, h, "POST", "/orders", validBody)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, want 400", rec.Code)
	}
	if strings.Contains(logBuf.String(), `"level":"ERROR"`) {
		t.Errorf("4xx 不该记 ERROR 级别的日志:\n%s\n"+
			"（只有 status >= 500 才记）", logBuf.String())
	}
}

// ---------- GET /orders/{id} ----------

func TestAPI_GetOrder(t *testing.T) {
	svc := &fakeSvc{order: &orders.Order{
		ID: 42, Customer: "bob", Status: orders.StatusPaid, TotalCents: 300,
		Items: []orders.Item{{ID: 1, SKU: "S", Qty: 3, PriceCents: 100}},
	}}
	h, _ := newTestRouter(svc, fakePinger{})

	rec := do(t, h, "GET", "/orders/42", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
	}
	if svc.gotID != 42 {
		t.Errorf("传给 service 的 id = %d, want 42", svc.gotID)
	}

	m := wantJSON(t, rec)
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items 不是数组: %v", m["items"])
	}
	if len(items) != 1 {
		t.Fatalf("items 有 %d 条, want 1", len(items))
	}
	it, _ := items[0].(map[string]any)
	if it["sku"] != "S" || it["qty"] != float64(3) {
		t.Errorf("item = %v", it)
	}
}

// TestAPI_GetOrderEmptyItemsIsArrayNotNull 抓 nil 切片序列化成 null（D12 §1.4）。
func TestAPI_GetOrderEmptyItemsIsArrayNotNull(t *testing.T) {
	svc := &fakeSvc{order: &orders.Order{ID: 1, Customer: "x", Status: orders.StatusPending}}
	h, _ := newTestRouter(svc, fakePinger{})

	rec := do(t, h, "GET", "/orders/1", "")
	body := rec.Body.String()

	if strings.Contains(body, `"items":null`) {
		t.Errorf("items 序列化成了 null，应该是 []:\n  %s\n"+
			"（切片的零值是 nil —— toResponse 里要初始化成空切片）", body)
	}
	m := wantJSON(t, rec)
	if _, ok := m["items"].([]any); !ok {
		t.Errorf("items = %#v, want []", m["items"])
	}
}

func TestAPI_GetOrderBadID(t *testing.T) {
	for _, path := range []string{"/orders/abc", "/orders/1.5", "/orders/9999999999999999999999"} {
		t.Run(path, func(t *testing.T) {
			svc := &fakeSvc{}
			h, _ := newTestRouter(svc, fakePinger{})

			rec := do(t, h, "GET", path, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("状态码 = %d, want 400", rec.Code)
			}
			if svc.gotID != 0 {
				t.Error("id 解析失败却还是调用了 service")
			}
			wantJSON(t, rec)
		})
	}
}

func TestAPI_GetOrderNotFound(t *testing.T) {
	h, _ := newTestRouter(&fakeSvc{err: apperr.NotFound("订单不存在", nil)}, fakePinger{})

	rec := do(t, h, "GET", "/orders/999", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("状态码 = %d, want 404", rec.Code)
	}
}

// ---------- GET /orders ----------

// TestAPI_ListFilterParsing 是查询参数解析的核心测试。
//
// ⭐ 重点在第 3、4 行：**参数没出现**和**参数是空字符串**必须区分开
// （D12 §1.2 的指针那套，换了个场景）。
func TestAPI_ListFilterParsing(t *testing.T) {
	str := func(s string) *string { return &s }

	tests := []struct {
		name     string
		path     string
		wantCust *string
		wantStat *string
		wantLim  int
	}{
		{"什么都不带", "/orders", nil, nil, 0},
		{"只有 customer", "/orders?customer=alice", str("alice"), nil, 0},
		{"customer 是空串", "/orders?customer=", str(""), nil, 0},
		{"两个条件", "/orders?customer=alice&status=paid", str("alice"), str("paid"), 0},
		{"带 limit", "/orders?limit=20", nil, nil, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeSvc{list: []orders.Order{}}
			h, _ := newTestRouter(svc, fakePinger{})

			rec := do(t, h, "GET", tt.path, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("状态码 = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
			}

			got := svc.gotFiler
			checkPtr(t, "Customer", got.Customer, tt.wantCust)
			checkPtr(t, "Status", got.Status, tt.wantStat)
			if got.Limit != tt.wantLim {
				t.Errorf("Limit = %d, want %d", got.Limit, tt.wantLim)
			}
		})
	}
}

func checkPtr(t *testing.T, name string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q，want nil（参数没出现时不该过滤）\n"+
			"（⚠️ 用 Query().Has(key) 判断，别用 Get(key) != \"\"）", name, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q（参数出现了就该过滤，哪怕值是空串）", name, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %q, want %q", name, *got, *want)
	}
}

func TestAPI_ListBadLimit(t *testing.T) {
	for _, path := range []string{"/orders?limit=abc", "/orders?limit=1.5", "/orders?limit="} {
		t.Run(path, func(t *testing.T) {
			svc := &fakeSvc{}
			h, _ := newTestRouter(svc, fakePinger{})

			rec := do(t, h, "GET", path, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("状态码 = %d, want 400\n"+
					"（⚠️ 别静默当成 0 —— 用户会拿到默认条数还以为参数生效了）", rec.Code)
			}
		})
	}
}

// TestAPI_ListUsesResponseDTO 抓「列表直接把领域对象序列化了」。
//
// ⚠️ 原来的 TestAPI_ListEmptyIsArrayNotNull 只测了【空列表】——
// 空数组里看不出字段名长什么样，所以这个 bug 完全溜过去了。
// ⭐ 又一次「测试数据太乖」（D6）：边界情况恰好掩盖了主路径的问题。
//
// 不走 DTO 的两个后果：
//
//  1. **字段名不一致** —— 单个订单是 id/customer，列表里是 ID/Customer，
//     客户端要写两套解析
//  2. ⚠️ **领域模型泄漏** —— 将来给 orders.Order 加个内部字段
//     （比如 InternalRiskScore），它会【自动出现在 API 响应里】
//
// 这正是讲义 §4 要防的事。
func TestAPI_ListUsesResponseDTO(t *testing.T) {
	svc := &fakeSvc{list: []orders.Order{
		{
			ID: 1, Customer: "alice", Note: "急", Status: orders.StatusPaid, TotalCents: 1000,
			Items: []orders.Item{{ID: 9, SKU: "A-1", Qty: 2, PriceCents: 500}},
		},
		{ID: 2, Customer: "bob", Status: orders.StatusPending, TotalCents: 50},
	}}
	h, _ := newTestRouter(svc, fakePinger{})

	rec := do(t, h, "GET", "/orders", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", rec.Code)
	}

	body := rec.Body.String()

	// ⚠️ 出现 Go 的字段名 = 直接序列化了 orders.Order
	for _, leaked := range []string{`"ID"`, `"Customer"`, `"Status"`, `"TotalCents"`, `"CreatedAt"`, `"Note"`} {
		if strings.Contains(body, leaked) {
			t.Errorf("响应里出现了 Go 的字段名 %s —— 列表没走 toResponse\n"+
				"（⭐ 每一条都要转成 orderResponse，别把 []orders.Order 直接扔给 json）\nbody: %s",
				leaked, body)
		}
	}

	m := wantJSON(t, rec)
	arr, ok := m["orders"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("orders 应该是 2 条的数组，得到 %v", m["orders"])
	}

	first, _ := arr[0].(map[string]any)
	// 和单个订单的响应保持【完全一致】的字段名
	for _, want := range []string{"id", "customer", "status", "total_cents", "created_at", "items"} {
		if _, ok := first[want]; !ok {
			t.Errorf("列表项缺少字段 %q（应该和 GET /orders/{id} 的形状一致）:\n  %v", want, first)
		}
	}
	if first["customer"] != "alice" || first["total_cents"] != float64(1000) {
		t.Errorf("列表项内容不对: %v", first)
	}
}

func TestAPI_ListEmptyIsArrayNotNull(t *testing.T) {
	h, _ := newTestRouter(&fakeSvc{list: []orders.Order{}}, fakePinger{})

	rec := do(t, h, "GET", "/orders", "")
	m := wantJSON(t, rec)

	arr, ok := m["orders"].([]any)
	if !ok {
		t.Fatalf("响应体应该是 {\"orders\":[...]}，得到 %v", m)
	}
	if len(arr) != 0 {
		t.Errorf("orders 有 %d 条, want 0", len(arr))
	}
	if strings.Contains(rec.Body.String(), `"orders":null`) {
		t.Error("空列表序列化成了 null，应该是 []")
	}
}

// ---------- pay / cancel ----------

func TestAPI_PayAndCancel(t *testing.T) {
	tests := []struct{ path, wantAct string }{
		{"/orders/5/pay", "pay"},
		{"/orders/5/cancel", "cancel"},
	}

	for _, tt := range tests {
		t.Run(tt.wantAct, func(t *testing.T) {
			svc := &fakeSvc{order: &orders.Order{ID: 5, Customer: "a", Status: orders.StatusPaid}}
			h, _ := newTestRouter(svc, fakePinger{})

			rec := do(t, h, "POST", tt.path, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("状态码 = %d, want 200\nbody: %s", rec.Code, rec.Body.String())
			}
			if svc.gotAct != tt.wantAct {
				t.Errorf("调用的是 %q, want %q", svc.gotAct, tt.wantAct)
			}
			if svc.gotID != 5 {
				t.Errorf("id = %d, want 5", svc.gotID)
			}
			// 应该返回更新后的订单
			m := wantJSON(t, rec)
			if m["id"] != float64(5) {
				t.Errorf("响应体里应该是更新后的订单，得到 %v", m)
			}
		})
	}
}

func TestAPI_PayConflict(t *testing.T) {
	h, _ := newTestRouter(
		&fakeSvc{err: apperr.Conflict("订单已经是 paid，不能再支付", nil)}, fakePinger{})

	rec := do(t, h, "POST", "/orders/1/pay", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("状态码 = %d, want 409", rec.Code)
	}
}

// TestAPI_PayWhenReloadFails 抓「pay 成功之后那次 Get 的错误被丢掉」。
//
// pay / cancel 的实现通常是：
//
//	svc.Pay(ctx, id)              // 改状态
//	o, _ := svc.Get(ctx, id)      // ⚠️ 再查回来返回给客户端
//	writeJSON(w, 200, toResponse(o))
//
// 如果那个 `_` 丢掉了错误，而 Get 恰好失败（数据库刚断、上游超时），
// `o` 就是 nil，`toResponse(o)` **直接空指针 panic**。
//
// ⚠️ 后果比 panic 本身更糟：Recover 中间件会兜成 500，
// 但**支付其实已经成功了** —— 客户端看到 500 会重试，于是重复支付。
//
// ⭐ lint 会报 "Error return value of svc.Get is not checked"。
// 和 D13 那个 tx.Commit() 是同一类：错误被 `_` 丢掉，后果是静默的数据不一致。
func TestAPI_PayWhenReloadFails(t *testing.T) {
	for _, action := range []string{"pay", "cancel"} {
		t.Run(action, func(t *testing.T) {
			svc := &reloadFailsSvc{}
			h, _ := newTestRouter(svc, fakePinger{})

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handler panic 了: %v\n"+
						"（⚠️ svc.Get 的错误被 `_` 丢掉了，o 是 nil）", r)
				}
			}()

			rec := do(t, h, "POST", "/orders/1/"+action, "")

			// 改状态成功了，但没法把结果读回来 —— 这是服务端的问题，5xx
			if rec.Code != http.StatusInternalServerError {
				t.Errorf("状态码 = %d, want 500\n"+
					"（读回订单失败要走 writeError，不能当成功返回）\nbody: %s",
					rec.Code, rec.Body.String())
			}
			wantJSON(t, rec)
		})
	}
}

// reloadFailsSvc：状态更新成功，但紧接着的 Get 失败。
type reloadFailsSvc struct{}

func (reloadFailsSvc) Place(context.Context, *orders.Order) error { return nil }
func (reloadFailsSvc) List(context.Context, orders.ListFilter) ([]orders.Order, error) {
	return []orders.Order{}, nil
}
func (reloadFailsSvc) Pay(context.Context, int64) error    { return nil } // ⭐ 成功
func (reloadFailsSvc) Cancel(context.Context, int64) error { return nil } // ⭐ 成功
func (reloadFailsSvc) Get(context.Context, int64) (*orders.Order, error) {
	// ⚠️ 这里【故意】不用 context.DeadlineExceeded 之类的 ctx 错误 ——
	// apperr.HTTPStatus 会先检查 ctx 错误（在 *Error 之前），
	// 那样即使包在 apperr.Internal 里也会映射成 504 而不是 500，
	// 这条测试就变成在考 ctx 的映射顺序了，跟它要抓的东西无关。
	//
	// 用一个普通的底层错误，映射路径才是 Kind → 状态码这一条。
	return nil, apperr.Internal("查询订单失败", errors.New("connection reset by peer"))
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	h, _ := newTestRouter(&fakeSvc{}, fakePinger{})

	rec := do(t, h, "DELETE", "/orders/1", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("状态码 = %d, want 405（Go 1.22 的路由会自动处理）", rec.Code)
	}
}

// ---------- /healthz ----------

// TestAPI_HealthzChecksDB 锁住「健康检查要真的检查」。
//
// ⚠️ 只返回 {"status":"ok"} 的健康检查毫无价值 —— 数据库挂了它照样绿，
// K8s 不会重启也不会摘流量，故障就一直挂着。
func TestAPI_HealthzChecksDB(t *testing.T) {
	t.Run("数据库正常", func(t *testing.T) {
		h, _ := newTestRouter(&fakeSvc{}, fakePinger{})
		rec := do(t, h, "GET", "/healthz", "")
		if rec.Code != http.StatusOK {
			t.Errorf("状态码 = %d, want 200", rec.Code)
		}
		wantJSON(t, rec)
	})

	t.Run("数据库挂了", func(t *testing.T) {
		h, _ := newTestRouter(&fakeSvc{}, fakePinger{err: errors.New("connection refused")})
		rec := do(t, h, "GET", "/healthz", "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("状态码 = %d, want 503\n"+
				"（⚠️ 健康检查必须真的 Ping 数据库 —— 只返回 {\"status\":\"ok\"} 等于没有）",
				rec.Code)
		}
	})
}

// TestAPI_HealthzDoesNotLeakDBError 确认健康检查也不泄漏内部细节。
func TestAPI_HealthzDoesNotLeakDBError(t *testing.T) {
	const secret = "dial tcp 10.0.0.5:5432: connection refused"
	h, _ := newTestRouter(&fakeSvc{}, fakePinger{err: errors.New(secret)})

	rec := do(t, h, "GET", "/healthz", "")
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("健康检查响应泄漏了数据库地址:\n  %s\n"+
			"（⚠️ /healthz 通常是【不鉴权】的，泄漏内网拓扑是安全问题）", rec.Body.String())
	}
}
