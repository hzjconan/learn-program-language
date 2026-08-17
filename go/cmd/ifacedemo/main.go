// Command ifacedemo 是 lessons/D5.md 的可运行验证。
//
// 核心要证明一件事：**接口值是 (动态类型, 动态值) 的二元组**。
// 理解了这个二元组，"nil 接口 != 装着 nil 指针的接口" 这个 Go 头号坑就不再神秘 ——
// 它是二元组定义的直接推论，而不是什么特殊规则。
//
//	go run ./cmd/ifacedemo
package main

import (
	"fmt"
	"strings"
)

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

// ---------- ① 隐式实现：没有 implements ----------

// Sounder 是「会发声」这个能力。
type Sounder interface{ Sound() string }

// Dog 是一个普通 struct，它【没有声明】自己实现了任何接口。
type Dog struct{ Name string }

// Sound 让 Dog 碰巧满足了 Sounder。
func (d Dog) Sound() string { return "汪！" }

// Robot 和 Dog 毫无关系，也没有共同基类。
type Robot struct{ ID int }

// Sound 让 Robot 也满足了 Sounder。
func (r Robot) Sound() string { return "哔哔——" }

func demoImplicit() {
	title("① 隐式实现：接口可以【后加】")
	// 这个切片里两种类型没有任何共同祖先，也没写过一个 implements。
	for _, s := range []Sounder{Dog{Name: "阿黄"}, Robot{ID: 7}} {
		fmt.Printf("  %-12T → %s\n", s, s.Sound())
	}
	fmt.Println("  ✅ Dog 和 Robot 的源码里没有任何一处提到 Sounder")
	fmt.Println("     在 Java 里，加一个接口要改所有实现类；Go 里改零行。")
}

// ---------- ② 接口值 = (动态类型, 动态值) ----------

// show 把接口的两格内容和 == nil 的结果打成一行。
//
// 注意值那一格先用 Sprintf 转成 string 再对齐 —— 直接对 struct/slice 用 %-10v，
// 宽度会作用到【每个字段】上，排出来是错位的。
func show(expr string, s Sounder) {
	fmt.Printf("  %-18s → 类型=%-12T 值=%-8s s==nil? %v\n",
		expr, s, fmt.Sprintf("%v", s), s == nil)
}

func demoIfaceValue() {
	title("② 接口值是一个二元组")

	var s Sounder // 零值接口
	show("var s Sounder", s)

	s = Dog{Name: "阿黄"}
	show("s = Dog{}", s)

	var p *Dog // nil 指针
	s = p
	show("s = (*Dog)(nil)", s)

	fmt.Println()
	fmt.Println("  ⚠️ 最后一行：值是 <nil>，但接口不等于 nil —— 因为【类型】那一格非空。")
	fmt.Println("     接口只有在 (类型, 值) 【两格都空】时才 == nil。")
	fmt.Printf("     调试接口问题时 %%v 会骗你，一定要一起打 %%T。\n")
}

// ---------- ③ 小接口：一个方法撑起整个 IO 生态 ----------

// upperWriter 实现 io.Writer：把写入的内容转成大写再存起来。
//
// 只要实现这一个方法，它就能接在 fmt.Fprintf、io.Copy、json.Encoder…… 后面。
type upperWriter struct{ sb strings.Builder }

// Write 实现 io.Writer。
func (w *upperWriter) Write(p []byte) (int, error) {
	w.sb.WriteString(strings.ToUpper(string(p)))
	return len(p), nil // 必须返回【传入的长度】，否则调用方会认为写少了
}

func demoSmallInterface() {
	title("③ 小接口哲学：io.Writer 只有一个方法")

	var w upperWriter // 零值可用（D4 §1）
	if _, err := fmt.Fprintf(&w, "hello, %s!", "gopher"); err != nil {
		fmt.Println("  写入失败:", err)
		return
	}
	fmt.Println(" ", w.sb.String())
	fmt.Println("  ✅ 实现 1 个方法，就接上了 fmt / io / json / gzip 的全部生态")
	fmt.Println("     接口越小，能塞进去的类型越多。")
}

// ---------- ④ nil 接口 != 装着 nil 指针的接口 —— 今天的题眼 ----------

// MyErr 是一个典型的自定义错误类型（指针接收者，D4 §3）。
type MyErr struct{ Code int }

// Error 实现 error。注意它没有解引用 e 之外的东西，所以 nil 接收者也不会 panic。
func (e *MyErr) Error() string { return fmt.Sprintf("出错了，code=%d", e.Code) }

// badMayFail 是【错误写法】：返回值类型写成了具体类型的变量。
//
// 好消息：这个坑有工具兜底。写这个演示时 golangci-lint 当场报了
//
//	SA4023: ...badMayFail never returns a nil interface value (staticcheck)
//
// 和 D4 那个 SA4005 一样 —— **只要你跑 make check，这类经典 bug 抓得住**。
// 这里加 nolint 是因为我们【故意】要它错，好让你看到运行时的表现。
//
//nolint:staticcheck // 故意演示 nil 接口坑
func badMayFail(shouldFail bool) error {
	var e *MyErr // nil 指针
	if shouldFail {
		e = &MyErr{Code: 500}
	}
	return e // ⚠️ 无论 shouldFail 是什么，返回的接口都非 nil
}

// goodMayFail 是【正确写法】：不出错时显式 return nil。
func goodMayFail(shouldFail bool) error {
	if shouldFail {
		return &MyErr{Code: 500}
	}
	return nil // ✅ 真正的 nil 接口 (nil, nil)
}

//nolint:staticcheck // 同上，故意让 err == nil 恒为 false
func demoNilInterface() {
	title("④ nil 接口 != 装着 nil 指针的接口（面试出现率第一）")

	err := badMayFail(false) // 没有失败
	fmt.Println("  【错误写法】badMayFail(false):")
	fmt.Printf("      fmt.Println(err) → %v          ← 看起来就是 nil\n", err)
	fmt.Printf("      err == nil       → %v        ← 但它不是！\n", err == nil)
	fmt.Printf("      %%T               → %T   ← 动态类型非空，所以接口非空\n", err)
	fmt.Println("      调用方的 if err != nil 会成立，然后处理一个不存在的错误。")

	fmt.Println()
	err = goodMayFail(false)
	fmt.Println("  【正确写法】goodMayFail(false):")
	fmt.Printf("      err == nil       → %v         ✅\n", err == nil)
	fmt.Printf("      %%T               → %T          ← 类型那一格是空的\n", err)

	fmt.Println()
	fmt.Println("  记住两条规则：")
	fmt.Println("    1. 函数返回值类型写 error，别写 *MyErr")
	fmt.Println("    2. 中间变量是具体类型时，别写 return f()，要先判断再 return")
	fmt.Println()
	fmt.Println("  🛠 好消息：staticcheck 会报 SA4023「never returns a nil interface value」，")
	fmt.Println("     所以只要跑 make check，这个坑抓得住。上面两个函数加了 nolint 才编过 lint。")
}

// ---------- ⑤ 类型断言与可选接口 ----------

// Closer 用来演示「可选接口」：不是所有 Sounder 都有它。
type Closer interface{ Close() error }

// LoudDog 除了会叫，还额外实现了 Close。
type LoudDog struct{ Dog }

// Close 实现 Closer。
func (LoudDog) Close() error { return nil }

// tryClose 探测 s 有没有额外实现 Closer —— 有就调，没有就算了。
//
// 这就是 http.ResponseWriter 探测 http.Flusher 用的模式：
// 基础能力放接口里，增强能力用断言探测。
func tryClose(s Sounder) string {
	c, ok := s.(Closer) // ⭐ 永远用 comma-ok 形式，单值形式失败会 panic
	if !ok {
		return "不支持 Close"
	}
	if err := c.Close(); err != nil {
		return "Close 失败: " + err.Error()
	}
	return "已 Close"
}

func demoAssertion() {
	title("⑤ 类型断言：comma-ok 与可选接口")

	var v any = "hello"
	if s, ok := v.(string); ok {
		fmt.Printf("  v.(string) → ok=true  s=%q\n", s)
	}
	if n, ok := v.(int); !ok {
		fmt.Printf("  v.(int)    → ok=false n=%d   ← 失败时拿到零值，不 panic\n", n)
	}
	fmt.Println("  ⚠️ 单值形式 v.(int) 会直接 panic；lint 也会拦（check-type-assertions）")

	fmt.Println()
	fmt.Printf("  Dog     → %s\n", tryClose(Dog{Name: "阿黄"}))
	fmt.Printf("  LoudDog → %s   ← 断言出了额外能力\n", tryClose(LoudDog{}))
}

// ---------- ⑥ type switch 的三个细节 ----------

func describe(v any) string {
	switch x := v.(type) {
	case nil: // nil 是一个独立的 case
		return "nil（注意：这里能匹配上，说明传进来的是真 nil 接口）"
	case int:
		return fmt.Sprintf("int，可以直接算术：x*2=%d", x*2)
	case string:
		return fmt.Sprintf("string，可以直接取长度：len=%d", len(x))
	case []int, []string:
		// ⚠️ 多类型 case 里 x 退化成 any，不能直接当 slice 用
		return fmt.Sprintf("某种切片，但 x 在这里的静态类型是 %T 之外的 any", x)
	case error: // 接口也能当 case，但要放在具体类型后面
		return "error：" + x.Error()
	default:
		return fmt.Sprintf("其他类型 %T", x)
	}
}

func demoTypeSwitch() {
	title("⑥ type switch：x 在每个 case 里类型不同")
	for _, v := range []any{nil, 42, "hi", []int{1, 2}, &MyErr{Code: 404}, 3.14} {
		fmt.Printf("  %-14s → %s\n", fmt.Sprintf("%v", v), describe(v))
	}
	fmt.Println()
	fmt.Println("  ⚠️ 对自己定义的类型做 type switch，通常说明该给它们加个方法了。")
}

// ---------- ⑦ 泛型：什么时候比接口好 ----------

// Number 是一个【类型集】约束。波浪号 ~int 表示「底层类型是 int 的所有类型」,
// 这样 D1 里 type Celsius float64 这种具名类型也能用。
type Number interface {
	~int | ~int64 | ~float64
}

// Sum 对任意数字切片求和 —— 函数体对所有 T 完全一样，这就是该用泛型的信号。
func Sum[T Number](s []T) T {
	var total T // ⭐ 泛型里拿零值的标准写法：你没法写字面量
	for _, v := range s {
		total += v // 只因为 Number 约束允许 +，这行才编译得过
	}
	return total
}

// Celsius 用来验证 ~ 的作用。
type Celsius float64

func demoGenerics() {
	title("⑦ 泛型：函数体对所有类型都一样时用它")

	fmt.Println("  Sum([]int{1,2,3})          =", Sum([]int{1, 2, 3}))
	fmt.Println("  Sum([]float64{1.5, 2.5})   =", Sum([]float64{1.5, 2.5}))
	fmt.Println("  Sum([]Celsius{20, 3})      =", Sum([]Celsius{20, 3}), "  ← 靠 ~float64 才行")

	fmt.Println()
	fmt.Println("  判据：函数体里要不要根据类型做不同的事？")
	fmt.Println("    不要 → 泛型（Sum、Map、Filter、Stack[T]）")
	fmt.Println("    要   → 接口（Sounder、io.Writer、Store）")
}

func main() {
	demoImplicit()
	demoIfaceValue()
	demoSmallInterface()
	demoNilInterface()
	demoAssertion()
	demoTypeSwitch()
	demoGenerics()
}
