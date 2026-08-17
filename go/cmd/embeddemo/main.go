// Command embeddemo 是 lessons/D4.md 的可运行验证。
//
// 核心要证明一件事：**Go 的嵌入不是继承**。它只是「把内层的方法提升到外层」的
// 语法糖，没有虚函数表、没有动态派发、没有 override。Java 里最常用的
// 「模板方法模式」在这里会静默失效 —— 编译通过，行为不对。
//
//	go run ./cmd/embeddemo
package main

import "fmt"

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

// ---------- ① 方法提升：嵌入确实带来了便利 ----------

// Base 是被嵌入的类型。
type Base struct{ Name string }

// Label 会被提升到所有嵌入了 Base 的类型上。
func (b Base) Label() string { return "我是 " + b.Name }

// Child 嵌入了 Base（注意：只写类型名，不写字段名）。
type Child struct {
	Base
	Age int
}

func demoPromotion() {
	title("① 方法提升：外层直接用内层的方法和字段")
	c := Child{Base: Base{Name: "阿狗"}, Age: 3}
	fmt.Println("  c.Label() =", c.Label())     // 提升的方法
	fmt.Println("  c.Name    =", c.Name)        // 提升的字段
	fmt.Println("  c.Base.Name =", c.Base.Name) //nolint:staticcheck // 故意写全，展示提升前的原样
	fmt.Println("  ⚠️ 这只是语法糖：编译器把 c.Label() 改写成了 c.Base.Label()")
}

// ---------- ② 模板方法模式失效 —— 今天最重要的一节 ----------

// Animal 想扮演 Java 里的抽象基类。
type Animal struct{ Name string }

// Speak 是「模板方法」：它想调用「子类」实现的 Sound()。
func (a Animal) Speak() string { return a.Name + "说：" + a.Sound() }

// Sound 是「默认实现」，本意是让「子类」覆盖它。
func (a Animal) Sound() string { return "……（没有声音）" }

// Dog 嵌入 Animal，并定义了自己的 Sound。
type Dog struct{ Animal }

// Sound 看起来像是「覆盖」了 Animal.Sound。
func (d Dog) Sound() string { return "汪！" }

func demoNoOverride() {
	title("② 模板方法模式：编译通过，行为不对")
	d := Dog{Animal{Name: "阿黄"}}

	fmt.Println("  d.Sound()  =", d.Sound()) // 直接调，用的是 Dog 的
	fmt.Println("  d.Speak()  =", d.Speak()) // 通过提升的 Speak 调
	fmt.Println()
	fmt.Println("  ⚠️ d.Speak() 里的 a.Sound() 调到的是 Animal.Sound，不是 Dog.Sound！")
	fmt.Println("     因为 Speak 的接收者 a 的静态类型就是 Animal，")
	fmt.Println("     它【根本不知道】自己被嵌进了 Dog 里 —— 没有虚函数表，没有动态派发。")
	fmt.Println()
	fmt.Println("  在 Java 里同样的代码会输出「阿黄说：汪！」，这是两种语言的根本差别。")
}

// ---------- ③ 正确做法：用接口做多态 ----------

// Sounder 是「会发声」这个能力的抽象。
//
// 注意接口由【使用方】定义，不是实现方声明 —— Dog2 里没有任何
// "implements Sounder" 的字样，它只是碰巧有这个方法（D5 细讲）。
type Sounder interface{ Sound() string }

// SpeakVia 把「模板」变成一个普通函数，参数是接口。
func SpeakVia(name string, s Sounder) string { return name + "说：" + s.Sound() }

// Dog2 不嵌入任何东西，只实现 Sound。
type Dog2 struct{ Name string }

// Sound 实现 Sounder。
func (d Dog2) Sound() string { return "汪！" }

// Cat2 同上。
type Cat2 struct{ Name string }

// Sound 实现 Sounder。
func (c Cat2) Sound() string { return "喵~" }

func demoInterface() {
	title("③ 正确做法：把「可变的部分」抽成接口")
	fmt.Println(" ", SpeakVia("阿黄", Dog2{Name: "阿黄"}))
	fmt.Println(" ", SpeakVia("咪咪", Cat2{Name: "咪咪"}))
	fmt.Println("  ✅ 多态在 Go 里靠【接口】，不靠嵌入")
}

// ---------- ④ 值接收者 vs 指针接收者 ----------

// Counter 用来演示两种接收者的差别。
type Counter struct{ N int }

// IncVal 是值接收者：它拿到的是 Counter 的一份拷贝，所以这次自增毫无意义。
//
// 写这个演示时 golangci-lint 当场就报了：
//
//	SA4005: ineffective assignment to field Counter.N (staticcheck)
//
// 也就是说，**这个经典 bug 是有工具兜底的** —— 只要你跑 lint。
// 这里加 nolint 是因为我们【故意】要它错，好让你看到运行时的表现。
func (c Counter) IncVal() { c.N++ } //nolint:staticcheck // 故意演示值接收者改不到原对象

// IncPtr 是指针接收者：它能改到原对象。
func (c *Counter) IncPtr() { c.N++ }

func demoReceiver() {
	title("④ 值接收者拿到的是拷贝")
	c := Counter{}
	c.IncVal()
	fmt.Printf("  调用 3 次 IncVal 后... ")
	c.IncVal()
	c.IncVal()
	fmt.Printf("c.N = %d   ← 一次都没生效\n", c.N)

	c.IncPtr()
	c.IncPtr()
	c.IncPtr()
	fmt.Printf("  调用 3 次 IncPtr 后... c.N = %d   ← 生效了\n", c.N)

	fmt.Println()
	fmt.Println("  注意 c 是个值不是指针，为什么能调 c.IncPtr()？")
	fmt.Println("  因为 c 是【可寻址】的，编译器自动改写成 (&c).IncPtr()。")
	fmt.Println("  但这个自动取地址在【接口赋值】时不生效 —— 见 ⑤")
}

// ---------- ⑤ 方法集：接口赋值时不会自动取地址 ----------

// Greeter 只要求一个 Greet 方法。
type Greeter interface{ Greet() string }

// PtrGreeter 只用指针接收者实现 Greet。
type PtrGreeter struct{ Name string }

// Greet 是指针接收者方法。
func (p *PtrGreeter) Greet() string { return "你好，" + p.Name }

func demoMethodSet() {
	title("⑤ 方法集：谁能被赋值给接口")

	var g Greeter = &PtrGreeter{Name: "阿狗"} // ✅ *PtrGreeter 的方法集包含 Greet
	fmt.Println(" ", g.Greet())

	fmt.Println()
	fmt.Println("  但下面这行【编译不过】：")
	fmt.Println("      var g Greeter = PtrGreeter{Name: \"阿狗\"}")
	fmt.Println("      → PtrGreeter does not implement Greeter")
	fmt.Println("        (method Greet has pointer receiver)")
	fmt.Println()
	fmt.Println("  规则：T 的方法集只含【值接收者】方法；")
	fmt.Println("       *T 的方法集含【值接收者 + 指针接收者】方法。")
	fmt.Println("  直接调用时编译器会自动取地址，赋值给接口时【不会】。")
}

// ---------- ⑥ 名字冲突与遮蔽 ----------

// A 和 B 都有同名方法 Who。
type A struct{}

// Who 属于 A。
func (A) Who() string { return "A" }

// B 也有 Who。
type B struct{}

// Who 属于 B。
func (B) Who() string { return "B" }

// Both 同时嵌入 A 和 B —— 两个 Who 深度相同，直接调用会编译失败。
type Both struct {
	A
	B
}

// Outer 自己定义了 Who，遮蔽掉内层的。
type Outer struct{ A }

// Who 遮蔽了 A.Who —— 深度浅的赢。
func (Outer) Who() string { return "Outer（遮蔽了 A）" }

func demoConflict() {
	title("⑥ 名字冲突：深度优先，同深度必须显式")
	var b Both
	fmt.Println("  b.A.Who() =", b.A.Who())
	fmt.Println("  b.B.Who() =", b.B.Who())
	fmt.Println("  ⚠️ b.Who() 编译不过：ambiguous selector b.Who")

	var o Outer
	fmt.Println("  o.Who()   =", o.Who(), "  ← 外层深度更浅，赢")
	fmt.Println("  o.A.Who() =", o.A.Who(), "  ← 内层还在，只是要显式访问")
}

// ---------- ⑦ struct 的比较与拷贝 ----------

// Point 所有字段都可比较，所以 Point 本身可比较。
type Point struct{ X, Y int }

// WithSlice 含有 slice 字段 —— 整个 struct 变得不可比较。
type WithSlice struct {
	Name string
	Tags []string
}

func demoCompare() {
	title("⑦ struct 的比较与拷贝")
	p1, p2 := Point{1, 2}, Point{1, 2}
	fmt.Printf("  Point{1,2} == Point{1,2} → %v   ← 逐字段比较\n", p1 == p2)
	fmt.Println("  ⚠️ WithSlice 含 slice 字段 → 不可比较，w1 == w2 编译错误")
	fmt.Println("     （slice/map/func 不可比较，会传染给包含它们的 struct）")

	w1 := WithSlice{Name: "a", Tags: []string{"x"}}
	w2 := w1 // 拷贝
	w2.Name = "b"
	w2.Tags[0] = "改了"
	fmt.Printf("  w2 := w1 后改 w2：w1.Name=%q（独立） w1.Tags=%v（串了，浅拷贝）\n",
		w1.Name, w1.Tags)
}

func main() {
	demoPromotion()
	demoNoOverride()
	demoInterface()
	demoReceiver()
	demoMethodSet()
	demoConflict()
	demoCompare()
}
