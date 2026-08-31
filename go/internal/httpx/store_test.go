package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ---------- Store ----------

func TestHTTPX_StoreBasics(t *testing.T) {
	s := NewStore()

	if created := s.Put("a", "1"); !created {
		t.Error("首次 Put 应该返回 created=true")
	}
	if created := s.Put("a", "2"); created {
		t.Error("覆盖时应该返回 created=false")
	}
	if v, ok := s.Get("a"); !ok || v != "2" {
		t.Errorf("Get(\"a\") = %q, %v, want \"2\", true", v, ok)
	}
	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}
	if !s.Delete("a") {
		t.Error("Delete 已存在的键应该返回 true")
	}
	if s.Delete("a") {
		t.Error("Delete 不存在的键应该返回 false")
	}
}

func TestHTTPX_StoreConcurrent(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			k := string(rune('a' + i%26))
			s.Put(k, "v")
			s.Get(k)
			s.Len()
		})
	}
	wg.Wait()
}

// ---------- Router ----------

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req(method, path, body))
	return rec
}

// wantJSON 检查响应是 JSON 且能解析。
func wantJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json\n"+
			"（⚠️ 用了 http.Error 吗？它会把 Content-Type 覆盖成 text/plain）", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("响应不是合法 JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return m
}

func TestHTTPX_RouterGet(t *testing.T) {
	s := NewStore()
	s.Put("k1", "hello")
	h := NewRouter(s)

	rec := do(t, h, "GET", "/items/k1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", rec.Code)
	}
	m := wantJSON(t, rec)
	if m["key"] != "k1" || m["value"] != "hello" {
		t.Errorf("body = %v, want key=k1 value=hello", m)
	}
}

func TestHTTPX_RouterGetNotFound(t *testing.T) {
	rec := do(t, NewRouter(NewStore()), "GET", "/items/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("状态码 = %d, want 404", rec.Code)
	}
	m := wantJSON(t, rec)
	if m["error"] == nil || m["error"] == "" {
		t.Errorf("错误响应里应该有非空的 error 字段，得到 %v", m)
	}
}

func TestHTTPX_RouterPut(t *testing.T) {
	s := NewStore()
	h := NewRouter(s)

	// 新建 → 201
	rec := do(t, h, "PUT", "/items/k1", "v1")
	if rec.Code != http.StatusCreated {
		t.Errorf("新建时状态码 = %d, want 201", rec.Code)
	}
	m := wantJSON(t, rec)
	if m["key"] != "k1" || m["value"] != "v1" {
		t.Errorf("body = %v", m)
	}

	// 覆盖 → 200
	rec = do(t, h, "PUT", "/items/k1", "v2")
	if rec.Code != http.StatusOK {
		t.Errorf("覆盖时状态码 = %d, want 200", rec.Code)
	}

	if v, _ := s.Get("k1"); v != "v2" {
		t.Errorf("存储里的值 = %q, want v2", v)
	}
}

// TestHTTPX_RouterJSONEscaping 抓的是【手拼 JSON 字符串】。
//
// ⚠️ 早先的测试只用了 "hello" / "v1" 这种干净的值，对拼接实现是免疫的
// —— 又一次「测试数据太乖」（D6 那条）。含特殊字符的输入才有区分能力：
//
//	含双引号  → invalid character 'h' after object key:value pair
//	含换行    → invalid character '\n' in string literal
//	含反斜杠  → invalid character 'p' in string escape code
//	注入闭合  → 解析成功，但多出一个你从没打算返回的字段（JSON 注入）
//
// 用 encoding/json 就全都自动转义了。
func TestHTTPX_RouterJSONEscaping(t *testing.T) {
	tests := []struct{ name, value string }{
		{"含双引号", `he said "hi"`},
		{"含换行", "line1\nline2"},
		{"含反斜杠", `C:\path\to`},
		{"注入闭合", `x","injected":"yes`},
		{"中文和 emoji", "你好 🎉"},
		{"含尖括号", "<script>alert(1)</script>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewRouter(NewStore())

			rec := do(t, h, "PUT", "/items/k", tt.value)
			m := wantJSON(t, rec)
			if m["value"] != tt.value {
				t.Errorf("PUT 返回的 value = %#v\nwant %#v", m["value"], tt.value)
			}
			if _, injected := m["injected"]; injected {
				t.Errorf("响应里被注入了额外字段 —— 手拼 JSON 字符串了吗？\nbody: %s", rec.Body.String())
			}

			got := wantJSON(t, do(t, h, "GET", "/items/k", ""))
			if got["value"] != tt.value {
				t.Errorf("GET 返回的 value = %#v\nwant %#v", got["value"], tt.value)
			}
		})
	}
}

// TestHTTPX_RouterContentTypes 检查每条响应路径的 Content-Type。
//
// 204 是特例：RFC 9110 说它不能有 body，所以也不该声明 body 的类型。
func TestHTTPX_RouterContentTypes(t *testing.T) {
	s := NewStore()
	s.Put("exists", "v")
	h := NewRouter(s)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantJSON   bool
	}{
		{"200 GET", "GET", "/items/exists", "", 200, true},
		{"404 GET", "GET", "/items/nope", "", 404, true},
		{"201 PUT", "PUT", "/items/new", "v", 201, true},
		{"400 空 body", "PUT", "/items/k", "", 400, true},
		{"413 超大 body", "PUT", "/items/k", strings.Repeat("x", 2<<20), 413, true},
		{"404 DELETE", "DELETE", "/items/nope", "", 404, true},
		{"204 DELETE", "DELETE", "/items/exists", "", 204, false},
		{"200 health", "GET", "/health", "", 200, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("状态码 = %d, want %d", rec.Code, tt.wantStatus)
			}
			ct := rec.Header().Get("Content-Type")
			if tt.wantJSON {
				if !strings.HasPrefix(ct, "application/json") {
					t.Errorf("Content-Type = %q, want application/json\n（每条响应路径都要设，别漏掉某个分支）", ct)
				}
			} else {
				if ct != "" {
					t.Errorf("204 不该有 Content-Type（它不能有 body），得到 %q", ct)
				}
				if rec.Body.Len() != 0 {
					t.Errorf("204 不该有 body，得到 %q", rec.Body.String())
				}
			}
		})
	}
}

func TestHTTPX_RouterPutEmptyBody(t *testing.T) {
	rec := do(t, NewRouter(NewStore()), "PUT", "/items/k1", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("空 body 时状态码 = %d, want 400", rec.Code)
	}
	wantJSON(t, rec)
}

// TestHTTPX_RouterPutTooLarge 验证请求体大小限制（§6）。
func TestHTTPX_RouterPutTooLarge(t *testing.T) {
	big := strings.Repeat("x", 2<<20) // 2MB，超过 1MB 上限
	rec := do(t, NewRouter(NewStore()), "PUT", "/items/k1", big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("2MB body 时状态码 = %d, want 413\n"+
			"（用 http.MaxBytesReader 限制 1MB —— 不限制的话一个 10GB 请求能打爆内存）",
			rec.Code)
	}
}

func TestHTTPX_RouterDelete(t *testing.T) {
	s := NewStore()
	s.Put("k1", "v")
	h := NewRouter(s)

	rec := do(t, h, "DELETE", "/items/k1", "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("状态码 = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 不该有 body，得到 %q", rec.Body.String())
	}

	rec = do(t, h, "DELETE", "/items/k1", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("删不存在的键，状态码 = %d, want 404", rec.Code)
	}
}

func TestHTTPX_RouterHealth(t *testing.T) {
	s := NewStore()
	s.Put("a", "1")
	s.Put("b", "2")

	rec := do(t, NewRouter(s), "GET", "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, want 200", rec.Code)
	}
	m := wantJSON(t, rec)
	if m["status"] != "ok" {
		t.Errorf("status = %v, want ok", m["status"])
	}
	if n, _ := m["items"].(float64); int(n) != 2 {
		t.Errorf("items = %v, want 2", m["items"])
	}
}

// TestHTTPX_RouterMethodNotAllowed 验证新路由自动返回 405（§2）。
func TestHTTPX_RouterMethodNotAllowed(t *testing.T) {
	rec := do(t, NewRouter(NewStore()), "PATCH", "/items/k1", "x")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("状态码 = %d, want 405（Go 1.22 的路由会自动处理，不用自己写）", rec.Code)
	}
}

// TestHTTPX_RouterUsesRequestContext 验证 handler 用了 r.Context()。
//
// 传一个已取消的 ctx 进去，handler 应该能察觉。
func TestHTTPX_RouterUsesRequestContext(t *testing.T) {
	t.Skip("这条留给你自己实现：想想怎么验证 handler 真的读了 r.Context()。" +
		"（提示：内存 Store 用不上 ctx，所以这个断言其实很难写 —— " +
		"review 时我们聊聊「怎么测试一个不需要 ctx 的 handler 有没有用 ctx」）")
}
