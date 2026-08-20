package logx

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// sampleReport 造一份固定的报告，供 formatter 测试用。
func sampleReport(t *testing.T) *Report {
	t.Helper()
	r, err := Analyze(strings.NewReader(strings.Join([]string{
		"2026-08-18T10:00:00Z INFO api-gateway 15 GET /users/42 -> 200",
		"2026-08-18T10:00:01Z INFO api-gateway 23 GET /orders -> 200",
		"2026-08-18T10:00:02Z WARN api-gateway 150 slow query",
		"2026-08-18T10:00:03Z ERROR db 200 connection refused",
		"2026-08-18T10:00:04Z INFO db 5 reconnected",
		"坏行",
	}, "\n")))
	if err != nil {
		t.Fatalf("准备测试数据失败: %v", err)
	}
	return r
}

// ---------- TextFormatter ----------

// TestFormat_TextFormatterContent 只检查「事实在不在」，不检查排版 ——
// 排版是你的自由，但这些数字必须能被人看到。
func TestFormat_TextFormatterContent(t *testing.T) {
	var buf bytes.Buffer
	if err := (TextFormatter{Top: 2}).Format(&buf, sampleReport(t)); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}
	out := buf.String()

	if out == "" {
		t.Fatal("输出是空的")
	}

	wants := []struct {
		what string
		sub  string
	}{
		{"总条数", "5"},
		{"失败条数", "1"},
		{"级别 INFO", "INFO"},
		{"级别 ERROR", "ERROR"},
		{"服务 api-gateway", "api-gateway"},
		{"服务 db", "db"},
		{"最慢那条的服务名", "db"},
	}
	for _, w := range wants {
		if !strings.Contains(out, w.sub) {
			t.Errorf("输出里找不到%s（%q）。完整输出：\n%s", w.what, w.sub, out)
		}
	}

	// p50/p95/p99 三个数都要出现。5 个样本升序 5,15,23,150,200：
	// p50 → idx=ceil(2.5)-1=2 → 23ms；p95 → idx=ceil(4.75)-1=4 → 200ms
	for _, sub := range []string{"23", "200"} {
		if !strings.Contains(out, sub) {
			t.Errorf("输出里找不到分位数 %q。完整输出：\n%s", sub, out)
		}
	}
}

// TestFormat_TextFormatterDeterministic 抓的是 map 遍历顺序随机的坑（D3）。
// 按级别/服务输出时必须先排序，否则同样的输入每次跑结果不一样。
func TestFormat_TextFormatterDeterministic(t *testing.T) {
	r := sampleReport(t)
	f := TextFormatter{Top: 3}

	var first bytes.Buffer
	if err := f.Format(&first, r); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}

	for i := range 20 {
		var buf bytes.Buffer
		if err := f.Format(&buf, r); err != nil {
			t.Fatalf("第 %d 次 Format 失败: %v", i, err)
		}
		if buf.String() != first.String() {
			t.Fatalf("第 %d 次输出和第一次不一样 —— map 遍历顺序是随机的，输出前要排序\n"+
				"第一次:\n%s\n这一次:\n%s", i, first.String(), buf.String())
		}
	}
}

func TestFormat_TextFormatterTopZero(t *testing.T) {
	var buf bytes.Buffer
	if err := (TextFormatter{Top: 0}).Format(&buf, sampleReport(t)); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Top=0 只是不输出「最慢的 N 条」那一段，其余统计还是要输出")
	}
}

// ---------- JSONFormatter ----------

func TestFormat_JSONFormatterStructure(t *testing.T) {
	var buf bytes.Buffer
	if err := (JSONFormatter{Top: 2}).Format(&buf, sampleReport(t)); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n输出：\n%s", err, buf.String())
	}

	// 顶层字段
	for _, key := range []string{"total", "failed", "by_level", "by_service", "latency_ms", "slowest"} {
		if _, ok := got[key]; !ok {
			t.Errorf("JSON 里缺字段 %q。实际：%s", key, buf.String())
		}
	}

	if n, _ := got["total"].(float64); int(n) != 5 {
		t.Errorf("total = %v, want 5", got["total"])
	}
	if n, _ := got["failed"].(float64); int(n) != 1 {
		t.Errorf("failed = %v, want 1", got["failed"])
	}

	byLevel, _ := got["by_level"].(map[string]any)
	if n, _ := byLevel["INFO"].(float64); int(n) != 3 {
		t.Errorf("by_level[INFO] = %v, want 3（key 要用级别名字符串，不是数字）", byLevel["INFO"])
	}

	bySvc, _ := got["by_service"].(map[string]any)
	if n, _ := bySvc["api-gateway"].(float64); int(n) != 3 {
		t.Errorf("by_service[api-gateway] = %v, want 3", bySvc["api-gateway"])
	}

	lat, _ := got["latency_ms"].(map[string]any)
	for _, k := range []string{"p50", "p95", "p99"} {
		v, ok := lat[k]
		if !ok {
			t.Errorf("latency_ms 里缺 %q", k)
			continue
		}
		// 必须是数字，不能是 "23ms" 这种字符串
		if _, isNum := v.(float64); !isNum {
			t.Errorf("latency_ms[%q] = %#v，机器读的格式里耗时应该是毫秒整数，不带单位", k, v)
		}
	}
	if n, _ := lat["p50"].(float64); int(n) != 23 {
		t.Errorf("latency_ms[p50] = %v, want 23", lat["p50"])
	}

	slowest, ok := got["slowest"].([]any)
	if !ok {
		t.Fatalf("slowest 不是数组：%#v", got["slowest"])
	}
	if len(slowest) != 2 {
		t.Fatalf("len(slowest) = %d, want 2", len(slowest))
	}
	first, _ := slowest[0].(map[string]any)
	for _, k := range []string{"time", "level", "service", "latency_ms", "message"} {
		if _, ok := first[k]; !ok {
			t.Errorf("slowest[0] 里缺字段 %q：%#v", k, first)
		}
	}
	if s, _ := first["service"].(string); s != "db" {
		t.Errorf("最慢那条的 service = %v, want db", first["service"])
	}
	if s, _ := first["level"].(string); s != "ERROR" {
		t.Errorf("最慢那条的 level = %v, want \"ERROR\"（用级别名，不是数字）", first["level"])
	}
}

// TestFormat_JSONFormatterEmptySlowest 验证 Top<=0 时输出 [] 而不是 null。
//
// 为什么重要：前端拿到 null 要额外判空，拿到 [] 可以直接遍历。
// Go 里 nil slice 序列化成 null，空 slice 序列化成 [] —— 这个区别会一路传到调用方。
func TestFormat_JSONFormatterEmptySlowest(t *testing.T) {
	var buf bytes.Buffer
	if err := (JSONFormatter{Top: 0}).Format(&buf, sampleReport(t)); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}
	if strings.Contains(buf.String(), `"slowest":null`) ||
		strings.Contains(buf.String(), `"slowest": null`) {
		t.Errorf("slowest 输出成了 null，应该是 []。输出：\n%s", buf.String())
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("不是合法 JSON: %v", err)
	}
	if arr, ok := got["slowest"].([]any); !ok || len(arr) != 0 {
		t.Errorf("slowest = %#v, want 空数组", got["slowest"])
	}
}

func TestFormat_JSONFormatterIndent(t *testing.T) {
	var plain, indented bytes.Buffer
	r := sampleReport(t)
	if err := (JSONFormatter{Top: 1}).Format(&plain, r); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}
	if err := (JSONFormatter{Top: 1, Indent: true}).Format(&indented, r); err != nil {
		t.Fatalf("Format 失败: %v", err)
	}
	if indented.Len() <= plain.Len() {
		t.Error("Indent=true 的输出应该比不缩进的长")
	}
	if !strings.Contains(indented.String(), "\n") {
		t.Error("Indent=true 的输出里应该有换行")
	}
}

// ---------- 写入失败要传出去 ----------

var errWrite = errors.New("写坏了")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }

// TestFormat_FormattersPropagateWriteError 验证两个 formatter 都不吞写入错误。
// 写 HTTP 响应时对端断开就是这个场景，吞掉它你就永远不知道响应没发出去。
func TestFormat_FormattersPropagateWriteError(t *testing.T) {
	r := sampleReport(t)
	formatters := map[string]Formatter{
		"TextFormatter": TextFormatter{Top: 2},
		"JSONFormatter": JSONFormatter{Top: 2},
	}
	for name, f := range formatters {
		t.Run(name, func(t *testing.T) {
			if err := f.Format(failingWriter{}, r); err == nil {
				t.Error("写入失败时 Format 必须返回错误")
			}
		})
	}
}
