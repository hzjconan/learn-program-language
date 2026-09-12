//go:build integration

// Package apitest 是【端到端集成测试】（D15 §8②）。
//
// ⭐ 和 internal/api 的测试的根本区别：
//
//	internal/api        假 service + 假 Pinger  → 测 HTTP 契约
//	internal/apitest    【真】service + 【真】repository + 【真】数据库
//	                    只有 HTTP 传输层是 httptest
//
// 这一层专门抓 §2.2 那种「fake 比真实现更正确」的 bug ——
// D14 的 bug 5（Create 的 RETURNING 漏了 status）就是典型：
// 两个 fake 都「帮忙」把 Status 填成了 pending，所以单元测试永远抓不到，
// 只有真库能发现「下单返回的 status 是空字符串」。
package apitest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TODO(D15)：实现这一包。
//
// # 需要什么
//
//	TestMain          起 Postgres 容器 + 跑迁移（和 internal/orders 那份一样）
//	newTestServer(t)  组装真实的三层 + httptest.NewServer
//	                  orders.NewRepo(db) → ordersvc.New(repo) → api.NewRouter(svc, db, logger)
//
// ⚠️ 注意 api.NewRouter 的第二个参数是 Pinger —— *sql.DB 天然满足它，
// 直接传 db 就行（这就是 §2.4 小接口的好处）。
//
// # 至少要覆盖的主路径
//
//	① POST /orders            → 201，⭐ 断言返回的 JSON 里 status 是 "pending"
//	                             （这一条就是抓 D14 bug 5 的）
//	② GET  /orders/{id}       → 200，和 ① 返回的内容【完全一致】
//	                             ⭐ 「下单接口」和「查询接口」给客户端的数据必须一样
//	③ POST /orders/{id}/pay   → 200，status 变成 "paid"
//	④ POST /orders/{id}/pay   → 409（重复支付）
//	⑤ GET  /orders?status=paid → 200，⭐ 断言列表项的字段名是 snake_case
//	                             （这一条抓 D14 bug 4）
//
// # ⚠️ 别在这里重测单元测试已经覆盖的东西
//
// 这一层慢（要起容器），所以只测【组装起来才会暴露】的问题：
// 各层拼接、序列化、真实 SQL 的行为。
// 业务规则的各种分支（Qty 边界、SKU 重复…）留给 ordersvc 的单元测试 ——
// 那里毫秒级，而且能穷举。
//
// ⭐ 这就是测试金字塔：**不是「快的多写点」，是「让盲区互相覆盖」**（§2.4）。

func TestCreateOrder(t *testing.T) {
	srv := newTestServer(t)

	order := createOrder(t, srv)

	if order["status"] != "pending" {
		t.Fatalf("expected status: \"pending\", got %s", order["status"])
	}

	// json.Number底层是string，避免“order["total_cents"] != 1000”会转成float64丢精度
	tc, ok := order["total_cents"].(json.Number)
	if !ok {
		t.Fatalf("total_cents is not json.Number")
	}

	if tc != "1000" {
		t.Fatalf("expected total cents: 1000, got %v", tc)
	}
}

func TestGetOrder(t *testing.T) {
	srv := newTestServer(t)

	createdOrder := createOrder(t, srv)
	id, ok := createdOrder["id"].(json.Number)
	if !ok {
		t.Fatalf("id is not json.Number")
	}

	req, err := http.NewRequestWithContext(t.Context(), "GET", srv.URL+"/orders/"+string(id), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", resp.StatusCode)
	}

	gotOrder := make(map[string]any)
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&gotOrder); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if !reflect.DeepEqual(createdOrder, gotOrder) {
		t.Fatalf("expected order to be equal, GET API returns %v, CREATE API returns %v", gotOrder, createdOrder)
	}

}

func TestPayOrder_Success(t *testing.T) {
	srv := newTestServer(t)

	createdOrder := createOrder(t, srv)
	id, ok := createdOrder["id"].(json.Number)
	if !ok {
		t.Fatalf("id is not json.Number")
	}

	req, err := http.NewRequestWithContext(t.Context(), "POST", srv.URL+"/orders/"+string(id)+"/pay", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", resp.StatusCode)
	}

	paidOrder := make(map[string]any)
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&paidOrder); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if paidOrder["status"] != "paid" {
		t.Fatalf("expected status: \"paid\", got %s", paidOrder["status"])
	}
}

func TestPayOrder_DuplicatedPay(t *testing.T) {
	srv := newTestServer(t)

	createdOrder := createOrder(t, srv)
	id, ok := createdOrder["id"].(json.Number)
	if !ok {
		t.Fatalf("id is not json.Number")
	}

	req, err := http.NewRequestWithContext(t.Context(), "POST", srv.URL+"/orders/"+string(id)+"/pay", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 不需要 body（只看状态码）：显式丢弃
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", resp.StatusCode)
	}

	// 重复支付，应该返回http status code 409
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 不需要 body（只看状态码）：显式丢弃
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected status code 409, got %d", resp.StatusCode)
	}
}

func TestListOrder_Paid(t *testing.T) {
	srv := newTestServer(t)

	createdOrder := createOrder(t, srv)
	id, ok := createdOrder["id"].(json.Number)
	if !ok {
		t.Fatalf("id is not json.Number")
	}

	req, err := http.NewRequestWithContext(t.Context(), "POST", srv.URL+"/orders/"+string(id)+"/pay", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code 200, got %d", resp.StatusCode)
	}

	// list paid orders
	req, err = http.NewRequestWithContext(t.Context(), "GET", srv.URL+"/orders?status=paid", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200\nbody: %s", resp.StatusCode, bodyBytes)
	}

	ordersResponse := make(map[string][]map[string]any)
	dec := json.NewDecoder(bytes.NewReader(bodyBytes))
	dec.UseNumber()
	if err := dec.Decode(&ordersResponse); err != nil {
		t.Fatalf("解析响应失败: %v\nbody: %s", err, bodyBytes)
	}

	if len(ordersResponse["orders"]) != 1 {
		t.Fatalf("expected 1 paid order, got %d", len(ordersResponse["orders"]))
	}
	first := ordersResponse["orders"][0]
	if first["status"] != "paid" {
		t.Errorf("Status = %q, want paid", first["status"])
	}
	if first["customer"] != "alice" {
		t.Errorf("Customer = %q, want alice", first["customer"])
	}
	// json.Number底层是string，避免“order["total_cents"] != 1000”会转成float64丢精度
	tc, ok := first["total_cents"].(json.Number)
	if !ok {
		t.Fatalf("total_cents is not json.Number")
	}
	if tc != "1000" {
		t.Errorf("TotalCents = %v, want 1000", tc)
	}
}

func createOrder(t *testing.T, srv *httptest.Server) map[string]any {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), "POST", srv.URL+"/orders", strings.NewReader(`{"customer":"alice","items":[{"sku":"A-1","qty":2,"price_cents":500}]}`))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 断言状态码是 201
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status code 201, got %d", resp.StatusCode)
	}

	order := make(map[string]any)
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&order); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	return order
}
