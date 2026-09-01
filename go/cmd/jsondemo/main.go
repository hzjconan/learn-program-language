// Command jsondemo 演示 encoding/json 的五个坑和 log/slog 的用法（D12 §1、§3）。
//
//	go run ./cmd/jsondemo
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func main() {
	title("一、encoding/json 的坑")
	numberPrecision()
	zeroVsMissing()
	pointerDistinguishes()
	unknownFields()
	omitemptyOnStruct()
	durationInJSON()

	title("二、log/slog")
	handlers()
	attrStyles()
	withAndGroup()
	ctxAttrs()
	levelFilter()
}

// ---------- 一、JSON ----------

// numberPrecision：JSON 数字默认解析成 float64，超过 2^53 静默丢精度。
func numberPrecision() {
	section("① 数字精度：int64 过一遍 any 就废了")

	const id int64 = 9007199254740993 // 2^53 + 1
	b := mustMarshal(map[string]int64{"id": id})

	var loose map[string]any
	mustUnmarshal(b, &loose)

	fmt.Printf("  原始 int64:      %d\n", id)
	fmt.Printf("  序列化:          %s\n", b)
	fmt.Printf("  解析成 any:      %.0f  ← ⚠️ 变了！\n", loose["id"])
	fmt.Printf("  类型是:          %T\n", loose["id"])

	// 解法 ①：解析到具体类型
	var strict struct {
		ID int64 `json:"id"`
	}
	mustUnmarshal(b, &strict)
	fmt.Printf("  解析到 int64 字段: %d  ✅\n", strict.ID)

	// 解法 ②：json.Number
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var num map[string]any
	must(dec.Decode(&num))
	// ⭐ 用【带 ok 的】类型断言（D5）—— 本仓库的 lint 开了 check-type-assertions，
	// 单值形式 num["id"].(json.Number) 会被挡下来，因为它断言失败就 panic。
	jn, ok := num["id"].(json.Number)
	if !ok {
		panic(fmt.Sprintf("UseNumber 之后应该是 json.Number，得到 %T", num["id"]))
	}
	n, err := jn.Int64()
	must(err)
	fmt.Printf("  UseNumber():      %d  ✅\n", n)
}

// zeroVsMissing：显式传零值和字段缺失，结果一模一样。
func zeroVsMissing() {
	section("② 零值 vs 缺失，分不清")

	type Config struct {
		Port  int  `json:"port"`
		Debug bool `json:"debug"`
	}
	for _, in := range []string{`{"port":8080,"debug":true}`, `{"port":0,"debug":false}`, `{}`} {
		var c Config
		mustUnmarshal([]byte(in), &c)
		fmt.Printf("  %-28s → Port=%d Debug=%v\n", in, c.Port, c.Debug)
	}
	fmt.Println("  ⚠️ 后两行结果相同 —— 分不清「显式设成 0」和「没传」")
}

// pointerDistinguishes：指针能区分「没传」和「传了零值」。
func pointerDistinguishes() {
	section("③ 指针能区分「没传」和「传了零值」")

	type Patch struct {
		Retries *int `json:"retries"`
	}
	for _, in := range []string{`{"retries":0}`, `{"retries":3}`, `{}`} {
		var p Patch
		mustUnmarshal([]byte(in), &p)
		if p.Retries == nil {
			fmt.Printf("  %-16s → nil（字段【缺失】，不要动这个配置）\n", in)
		} else {
			fmt.Printf("  %-16s → %d（【显式传了】这个值）\n", in, *p.Retries)
		}
	}
}

// unknownFields：未知字段默认被静默忽略 —— 配置文件里拼错字段名查半天。
func unknownFields() {
	section("④ 未知字段默认被【静默忽略】")

	type Config struct {
		Port int `json:"port"`
	}
	const typo = `{"prot":9999}` // port 拼错了

	var c1 Config
	err := json.Unmarshal([]byte(typo), &c1)
	fmt.Printf("  默认:                 err=%v Port=%d  ← ⚠️ 静默吃掉了\n", err, c1.Port)

	var c2 Config
	dec := json.NewDecoder(strings.NewReader(typo))
	dec.DisallowUnknownFields()
	fmt.Printf("  DisallowUnknownFields: err=%v  ✅\n", dec.Decode(&c2))
}

// omitemptyOnStruct：omitempty 对 struct 无效。
func omitemptyOnStruct() {
	section("⑤ omitempty 的边界")

	type Inner struct{ X int }
	var v struct {
		A string `json:"a,omitempty"`
		B []int  `json:"b,omitempty"`
		C *Inner `json:"c,omitempty"`
		D Inner  `json:"d,omitempty"` // ⚠️ struct，omitempty 管不着
	}
	b := mustMarshal(v)
	fmt.Printf("  全零值 → %s\n", b)
	fmt.Println("  ⚠️ a/b/c 都省掉了，d 还在 —— omitempty 对【struct】无效")
	fmt.Println("     要省掉就改成指针（像 c 那样）")
}

// durationInJSON：time.Duration 在 JSON 里是纳秒整数。
func durationInJSON() {
	section("⑥ time.Duration 在 JSON 里是纳秒整数")

	b := mustMarshal(map[string]time.Duration{"took": 15 * time.Millisecond})
	fmt.Printf("  15ms → %s\n", b)
	fmt.Println("  给人看的话自己转：TookMs int64 `json:\"took_ms\"`（D7：边界做转换）")
}

// ---------- 二、slog ----------

// fixedTime 把时间戳固定住，让 demo 输出可复现。
func fixedTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		return slog.String("time", "2026-09-01T10:00:00Z")
	}
	return a
}

func newText(lvl slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{Level: lvl, ReplaceAttr: fixedTime}))
}

func newJSON() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout,
		&slog.HandlerOptions{ReplaceAttr: fixedTime}))
}

func handlers() {
	section("① 同一条日志，两种 Handler")
	newText(slog.LevelInfo).Info("请求完成", "method", "GET", "status", 200, "took", 15*time.Millisecond)
	newJSON().Info("请求完成", "method", "GET", "status", 200, "took", 15*time.Millisecond)
	fmt.Println("  开发用 Text（人读），生产用 JSON（机器读）—— 应该是个配置项")
}

func attrStyles() {
	section("② 强类型 attr vs 键值对")
	l := newJSON()
	l.Info("强类型（快，热路径用这个）", slog.String("method", "GET"), slog.Int("status", 200))
	l.Info("键值对（方便，走反射）", "method", "GET", "status", 200)

	// ⚠️ 键值对形式的参数个数必须是偶数，落单的会变成 "!BADKEY"。
	//
	// 直接写 l.Info("...", "method") 的话 go vet 会报
	// 「call to slog.Logger.Info missing a final value」—— 很好。
	// 但只要参数是动态拼出来的，vet 就看不见了，这里演示的正是那种情况。
	args := []any{"method"} // 少了对应的值
	l.Info("参数落单了", args...)

	fmt.Println("  ⚠️ 最后一条变成了 !BADKEY")
	fmt.Println("     字面量形式 go vet 能抓；动态拼出来的抓不到 —— 尽量用 slog.String 这种强类型")
}

func withAndGroup() {
	section("③ With：把公共字段固定下来")
	reqLog := newJSON().With("request_id", "abc123", "user", "alice")
	reqLog.Info("开始处理")
	reqLog.Warn("命中限流")
	fmt.Println("  ⭐ 每个请求做一个带上下文的 logger，比每条日志手动重复好得多")

	section("④ Group：给字段分组")
	newJSON().Info("分组", slog.Group("http",
		slog.String("method", "POST"), slog.Int("status", 201)))
}

// ---- ⑤ 从 ctx 自动提取字段 ----

type traceKey struct{}

// ctxHandler 包住任意 Handler，把 ctx 里的 trace ID 自动加到每条日志上。
//
// 嵌入 slog.Handler，只重写 Handle —— 和 D11 §5 的 responseRecorder 是同一招。
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := ctx.Value(traceKey{}).(string); ok {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func ctxAttrs() {
	section("⑤ 从 ctx 自动提取字段（最实用的扩展点）")

	l := slog.New(ctxHandler{slog.NewJSONHandler(os.Stdout,
		&slog.HandlerOptions{ReplaceAttr: fixedTime})})
	ctx := context.WithValue(context.Background(), traceKey{}, "trace-xyz")

	l.InfoContext(ctx, "带 ctx 的日志") // ⭐ 自动带上 trace_id
	l.Info("不带 ctx 的日志")            // 提取不到
	fmt.Println("  ⚠️ 必须用 XxxContext 系列 —— 普通的 Info 传的是 Background()")
}

func levelFilter() {
	section("⑥ 级别过滤")
	quiet := newText(slog.LevelWarn)
	quiet.Debug("这条不会出现")
	quiet.Info("这条也不会")
	quiet.Warn("这条会")
}

// ---------- 输出辅助 ----------

func title(s string) {
	fmt.Printf("\n\n%s\n%s\n", s, strings.Repeat("=", 60))
}

func section(s string) { fmt.Printf("\n--- %s ---\n", s) }

// ---------- Must 系列 ----------
//
// 这是 Go 里处理「按构造就不可能失败」的惯用法（标准库的 regexp.MustCompile、
// template.Must 都是这个路子）：失败就 panic，而不是把 error 一路传上去。
//
// ⚠️ 只在【输入是写死的字面量】时才这么用。凡是来自网络、文件、用户的输入，
// 一律老老实实处理 error。
//
// 顺带一提：本仓库的 errcheck 开了 check-blank，`_ = json.Unmarshal(...)`
// 这种「用空白标识符丢掉 error」的写法也会被 lint 挡下来 —— 逼你想清楚
// 到底是「不可能失败」还是「懒得处理」。

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	must(err)
	return b
}

func mustUnmarshal(b []byte, v any) {
	must(json.Unmarshal(b, v))
}
