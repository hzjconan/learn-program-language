// Command jsondemo 演示 encoding/json 的七个坑和 log/slog 的用法（D12 §1、§3）。
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
	omitzeroTag()
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

	// 解法 ①：让接住数字的是【具体类型】。
	//
	// ⚠️ 起决定作用的是「目标类型具不具体」，不是 struct 还是 map ——
	// map[string]int64 和 struct{ID int64} 走的是同一条路径，都精确。
	// 目标是 int64 时 json 直接 ParseInt 原始字节，全程没有 float64 出场。
	var strict struct {
		ID int64 `json:"id"`
	}
	mustUnmarshal(b, &strict)
	fmt.Printf("  解析到 int64 字段: %d  ✅\n", strict.ID)

	var mi map[string]int64
	mustUnmarshal(b, &mi)
	fmt.Printf("  map[string]int64: %d  ✅ （map 一样精确）\n", mi["id"])

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

	// ⭐ any 不只是丢精度 —— 它还让 json 失去了【拒绝坏数据】的能力。
	const overflow = `{"id":99999999999999999999}` // 远超 int64 范围
	var mi2 map[string]int64
	fmt.Printf("\n  溢出值解析成 int64: %v\n", json.Unmarshal([]byte(overflow), &mi2))
	var ma map[string]any
	err = json.Unmarshal([]byte(overflow), &ma)
	fmt.Printf("  溢出值解析成 any:   err=%v，值=%v  ⚠️ 不报错\n", err, ma["id"])
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

// Inner 是下面几个例子共用的内嵌结构体。
type Inner struct{ X int }

// omitemptyOnStruct：omitempty 对 struct 无效，以及指针解法。
func omitemptyOnStruct() {
	section("⑤ omitempty 的边界：对 struct 无效")

	var v struct {
		A string `json:"a,omitempty"`
		B []int  `json:"b,omitempty"`
		C *Inner `json:"c,omitempty"`
		D Inner  `json:"d,omitempty"` // ⚠️ struct，omitempty 管不着
	}
	fmt.Printf("  全零值 → %s\n", mustMarshal(v))
	fmt.Println("  ⚠️ a/b/c 都省掉了，d 还在 —— omitempty 对【struct】无效")
	fmt.Println()
	fmt.Println("  omitempty 只认这几种「空值」：false / 0 / \"\" / nil / 空切片 / 空 map。")
	fmt.Println("  struct【不在列表里】，哪怕它所有字段都是零值。")

	// ---- 解法一：改成指针 ----
	section("⑤b 解法一：改成指针")

	type withPtr struct {
		D *Inner `json:"d,omitempty"`
	}
	type withValue struct {
		D Inner `json:"d,omitempty"`
	}

	// ⚠️ 被 %-Ns 填充的字段一律用【纯 ASCII】—— %-Ns 按字节数填充，
	// 而中文一个字 3 字节却只占 2 显示列，混着写必然错位（D7 那条）。
	// 中文注解放在行尾，不参与对齐。
	row := func(label string, v any, note string) {
		fmt.Printf("    %-12s → %-20s %s\n", label, mustMarshal(v), note)
	}

	fmt.Println("  值类型（对照组）:")
	row("Inner{}", withValue{}, "← 零值也被输出了 ⚠️")
	row("Inner{X:7}", withValue{D: Inner{X: 7}}, "")

	fmt.Println("  指针:")
	row("nil", withPtr{}, "← 省掉了 ✅")
	row("&Inner{}", withPtr{D: &Inner{}}, "← 零值但【存在】，保留")
	row("&Inner{X:7}", withPtr{D: &Inner{X: 7}}, "")

	fmt.Println()
	fmt.Println("  ⭐ 注意中间那行：&Inner{} 是零值，但它【被保留了】。")
	fmt.Println("     指针区分的是「有没有」，不是「是不是零值」—— 和 ③ 是同一个道理。")
	fmt.Println("     所以指针不只是「让 omitempty 生效」的小把戏，它换了一种语义：")
	fmt.Println("       nil       = 这个字段【不存在】")
	fmt.Println("       &Inner{}  = 存在，值恰好是零")
	fmt.Println("     代价：每次读都要判空。只在真的需要区分时才用。")
}

// omitzeroTag：Go 1.24 的 omitzero —— 值类型 struct 也能省掉。
func omitzeroTag() {
	section("⑥ 解法二：omitzero（Go 1.24+）")

	type withOmitzero struct {
		D Inner `json:"d,omitzero"` // 注意：值类型，不是指针
	}
	row := func(label string, v any, note string) {
		fmt.Printf("    %-12s → %-20s %s\n", label, mustMarshal(v), note)
	}
	row("Inner{}", withOmitzero{}, "← 零值，省掉了 ✅")
	row("Inner{X:7}", withOmitzero{D: Inner{X: 7}}, "")
	fmt.Println("  不用改成指针，也不用判空 —— 只想「零值就别输出」的话，这个更省事。")

	// ---- 两者的差别，双向都有 ----
	fmt.Println()
	fmt.Println("  ⚠️ 但 omitzero 不是 omitempty 的超集，两者在【两个方向】上都不一样：")
	fmt.Println()

	var t1 struct {
		T time.Time `json:"t,omitempty"`
	}
	var t2 struct {
		T time.Time `json:"t,omitzero"`
	}
	fmt.Printf("    零值 time.Time + omitempty → %s\n", mustMarshal(t1))
	fmt.Printf("    零值 time.Time + omitzero  → %s\n", mustMarshal(t2))
	fmt.Println("    ⭐ 这是个真实事故：time.Time 是 struct，omitempty 对它无效，")
	fmt.Println("       于是「没设置过的时间」被当成【公元 1 年】发给了客户端。")

	fmt.Println()
	var s1 struct {
		S []int `json:"s,omitempty"`
	}
	var s2 struct {
		S []int `json:"s,omitzero"`
	}
	s1.S, s2.S = []int{}, []int{}
	fmt.Printf("    空切片 []int{} + omitempty → %s\n", mustMarshal(s1))
	fmt.Printf("    空切片 []int{} + omitzero  → %s\n", mustMarshal(s2))
	fmt.Println("    ⭐ 反过来了！omitzero 只认【零值】，而 []int{} 不是 nil，所以保留。")

	fmt.Println()
	fmt.Println("  记法：omitempty 问「空不空」，omitzero 问「是不是零值」。")
	fmt.Println("        对 struct 和 time.Time 用 omitzero；")
	fmt.Println("        想让空切片也输出成 [] 而不是消失，也用 omitzero。")
}

// durationInJSON：time.Duration 在 JSON 里是纳秒整数。
func durationInJSON() {
	section("⑦ time.Duration 在 JSON 里是纳秒整数")

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

// newText / newJSON 造两个默认级别（Info）的 logger，只差一个 Handler。
//
// ⚠️ 这两个签名【必须一致】。早先 newText 收一个 level 参数而 newJSON 不收 ——
// 纯粹因为当时只有 ⑥ 那节需要非默认级别，就顺手给用得着的那个加了参数。
// 这是「按当前调用方需要来定签名」，结果是一对不对称的 API：
// 读的人会以为「text 能调级别、json 不能」，而这个区别根本不存在。
//
// 需要非默认级别的地方（就 ⑥ 一处）自己写 HandlerOptions —— 那一节讲的
// 正是级别过滤，把 Level 摊在眼前比藏进 helper 参数里更清楚。
func newText() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{ReplaceAttr: fixedTime}))
}

func newJSON() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout,
		&slog.HandlerOptions{ReplaceAttr: fixedTime}))
}

func handlers() {
	section("① 同一条日志，两种 Handler")
	newText().Info("请求完成", "method", "GET", "status", 200, "took", 15*time.Millisecond)
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

	// ⭐ Level 就是这一节的主角，所以直接写出来，不藏进 helper。
	// 默认级别是 Info（零值 slog.LevelInfo == 0），所以想看 Debug
	// 必须显式设成 slog.LevelDebug —— 这也是最常见的「我的 Debug 日志
	// 怎么不输出」的原因。
	quiet := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:       slog.LevelWarn,
		ReplaceAttr: fixedTime,
	}))
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

// mustUnmarshal 把 JSON 解析进 v 指向的变量。
//
// ⭐ 注意签名是 `v *T` 而不是照抄 json.Unmarshal 的 `v any` —— 这是本文件
// 唯一一处刻意偏离标准库的地方，理由值得单独说：
//
// # 问题：`any` 说不出「必须传指针」
//
// json.Unmarshal(data []byte, v any) 要求 v 【必须是指针】（否则没法把结果
// 写回调用方的变量），但这个要求在签名里【完全看不出来】。用的人只能靠
// 读文档，或者等运行时炸：
//
//	json.Unmarshal(b, item)    → json: Unmarshal(non-pointer main.Item)
//	                             （编译通过，item 静静地没被填充）
//	json.Unmarshal(b, p)       → json: Unmarshal(nil *main.Item)
//
// 标准库里 errors.As 是同一个毛病，而且更狠 —— 直接 panic：
//
//	errors.As(err, target)     → panic: errors: target must be a non-nil pointer
//
// （所以你在 apperr 里写 errors.As 时，第二个参数一定要记得取地址。）
//
// # 为什么不能写成 `v *any`
//
// 直觉上想用 *any 表达「指向任意类型的指针」，但那是另一回事 ——
// *any 是【指向一个接口变量】的指针，Go 没有协变，*Item 转不过去：
//
//	cannot use &x (value of type *Item) as *any value:
//	  type *any is pointer to interface, not interface
//
// # 解法：泛型（D5）
//
// `v *T` 里的 *T 【明说】了要指针，忘了 & 就是编译错误，而不是运行时惊喜：
//
//	mustUnmarshal(b, x)   → type Item of x does not match *T (cannot infer T)
//
// T 由实参推断，调用处不用写类型参数，读起来和原来一模一样。
//
// # 那标准库为什么不改
//
// json.Unmarshal 是 Go 1.0 的 API，泛型是 1.18 才有的，改签名会破坏所有
// 现存代码。你写【新】代码没有这个包袱。
//
// ⭐ 一般性结论：**「必须传指针」这种约束，能用类型表达就别只写进文档** ——
// 文档要人读，类型让编译器强制。
func mustUnmarshal[T any](b []byte, v *T) {
	must(json.Unmarshal(b, v))
}
