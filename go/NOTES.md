# 学习笔记

> 每天结束写 3 行：**今天最反直觉的一点 / 踩的坑 / 还没搞懂的**。
> 不用写长，写"当时让你愣住的那一下"就行。回头复习时这份笔记比任何教程都值钱。

---

## D0 · 环境重建（2026-08-14）

- 反直觉：
- 踩的坑：
- 没搞懂：

---

## D1 · 工程骨架与类型系统（2026-08-14）

讲解见 `lessons/D1.md`，练习在 `internal/units/`。

**小题 B · 作用域谜题**

题目：先不要跑，用脑子推。

```go
func main() {
	x := 1
	{
		x := 2
		x++
		fmt.Println("inner:", x)
	}
	fmt.Println("outer:", x)

	y := 1
	if y := 10; y > 5 {
		fmt.Println("if:", y)
	}
	fmt.Println("after if:", y)
}
```

我的答案：

```
inner: 3
outer: 1
if: 10
after if: 1
```

为什么（说清楚哪几个 x / y 是不同的变量，各自活在哪个范围里）：
我的理解：第30行要打印的**x**其实是在28行定义的，括号**内部**的变量。到了第32行，**x**就变成了第26行定义的，括号**外部**的变量。

**y**也是一样的，第36行要打印的**y**其实是在35行if语句里定义的。到了第38行，**y**就变成了第34行定义的。

实际跑完的验证结果（对了/错在哪）：

- 反直觉：
- 踩的坑：
1. bytesize.go的第70行，现在用float64(n)/float64(u.size)是对的，之前用float64(n/u.size), n/u.size 是整数除法，会丢失精度。
2. bytesize.go的第22行，我一开始写的**B  = 1 + iota**这样会造成**B**丢失了它的类型**ByteSize**
- 没搞懂：
1. fmt.Sprintf里面的各种格式化参数不是很熟悉 → 已解决，速查表记在 QA.md #5

---

## D2 · 函数、错误处理、defer（2026-08-14）

讲解见 `lessons/D2.md`，练习在 `internal/kvconf/` 和 `internal/mathx/`。

**小题 A · defer 谜题**

四段代码，**先不要跑**，用脑子推。重点不是猜对，是说清楚「为什么」。

```go
// ① 循环里的 defer
func a() {
	for i := range 3 {
		defer fmt.Println("defer:", i)
	}
	fmt.Println("函数体结束")
}

// ② 值捕获 vs 闭包捕获
func b() {
	x := 1
	defer fmt.Println("值捕获:", x)
	defer func() { fmt.Println("闭包捕获:", x) }()
	x = 100
}

// ③ 命名返回值
func c() (n int) {
	defer func() { n *= 2 }()
	return 5
}

// ④ 匿名返回值（和 ③ 只差一个名字）
func d() int {
	n := 5
	defer func() { n *= 2 }()
	return n
}
```

我的答案：

```
① 
我认为的结果：
函数体结束 -> 2 -> 1 -> 0

我的解释：
defer在函数return语句后执行，并且是Last-In-First-Out的顺序，所以看上去是“倒序”输出

② 
我认为的结果：
闭包捕获: 100
值捕获: 1

我的解释：
值捕获是当前变量的值，所以在给x赋值成1的时候就定下来了。
闭包捕获是变量的引用，在函数最后阶段执行，这时候x的值已经被改成100了。

③ c() = 10

④ d() = 5
我的理解：
c函数定位返回的时候给了**n**这个变量名，所以defer函数里面，修改n的值，会影响到返回值。
d函数定位返回的时候没有给变量名，所以defer函数里面，修改n的值，不会影响返回值。
```

为什么（③ 和 ④ 的差别是今天的重点，请说清楚 `return` 的三个步骤）：


实际跑完的验证结果（对了/错在哪）：


- 反直觉：
- 踩的坑：
- 没搞懂：
1.函数的返回，返回的到底是**值**还是**引用**，比如上面的c或者d函数，返回的的值，到底是这个值本身，还是一个指向这个值的引用？再比如，我有一个函数t，函数体里面长这样：
{
	a := {...}//a是一个结构体， 假设已经定义过了
	return a //返回的这个a，是这个结构体本身，还是引用？换句话说，调用者拿到了这个返回值之后，go语言相当于是把这个结构体拷贝了一份呢？还是直接返回了它的引用？或者是它的引用的拷贝（这个拷贝的引用最终还是指向这个结构体）？
}
→ **已解决**：值。返回值和参数一样都是拷贝。但**只是浅拷贝**，slice/map/指针字段仍然共享底层数据。见 `lessons/D2.md` §2.5.1

2. **recover()**函数返回的到底是什么？
→ **已解决**：`any`，就是当初传给 `panic()` 的那个值原封不动。运行时 panic 传的是 `runtime.Error`（六种类型的实测表见讲义）。没 panic 时返回 nil。见 `lessons/D2.md` §7「运行时 panic vs 应用 panic」

3. 函数里面给变量赋值的时候，**:=**和**=**有什么区别？我在kvconf.go的ParseFile函数里，defer那块，因为不需要检查**f.Close()**出错值，所以用了**_ =**，但是如果写**_ :=**会报错。
→ **已解决**：`:=` 是声明+初始化，硬规则是「左边至少一个**新变量**」；`_` 不声明任何变量，所以 `_ :=` 左边一个新的都没有。见 `QA.md` #9

4. **errors.New**要求的参数是字符串，所以go里面的error本质上和string有啥区别？可以认为就是**另一种类型**的string吗？
→ **已解决**：不是。`error` 是接口，`errors.New` 只是把字符串塞进一个实现了 `Error() string` 的内部类型，字符串是它的**字段**不是它本身。差别：有身份、能携带任意数据、能形成 `Unwrap` 链。见 `lessons/D2.md` §4

5. （追问）`errors.Is` 比较的是内存地址吗？
→ **已解决**：不一定，是「接口相等」= 动态类型+动态值；还能被自定义 `Is` 方法接管（`fs.ErrNotExist` 的跨平台映射就靠这个）。见 `QA.md` #8

---

## D3 · slice / map / string（2026-08-16）

讲解见 `lessons/D3.md`，练习在 `internal/slicex/` 和 `internal/stringx/`。
先跑 `go run ./cmd/slicedemo` 建立直觉。

**小题 · slice 谜题**

三段代码，**先不要跑**，用脑子推。重点说清楚「底层数组有没有被共享」。

```go
// ① append 会不会踩到别人
func a() {
	arr := [5]int{1, 2, 3, 4, 5}
	s := arr[1:3]
	s = append(s, 100)
	fmt.Println("arr =", arr)
	fmt.Println("s   =", s)
}

// ② 两个切片指向同一块内存
func b() {
	s := []int{1, 2, 3, 4, 5}
	x := s[1:3]
	y := s[2:4]
	x[1] = 99
	fmt.Println("s =", s)
	fmt.Println("x =", x)
	fmt.Println("y =", y)
}

// ③ 函数里的 append，调用方看得见吗
func c() {
	s := make([]int, 3, 10)   // 注意 cap 是 10
	modify(s)
	fmt.Println("s =", s, "len =", len(s))
}
func modify(v []int) {
	v = append(v, 999)
	v[0] = 111
}
```

我的答案：

```
① arr = 1 2 3 100 5
   s   = 2 3 100

② s = 1 2 99 4 5
   x = 2 99
   y = 99 4

③ s = 111 0 0 len = 3
```

为什么（每一问都说清楚「谁和谁共享底层数组」）：
① arr 是一个数组，s 是一个切片，底层数据结构是:
[
  &arr, //指向arr的指针
  2, //s的长度
  4, //s的容量
]
调用了append(s, 100)后，s的长度变成了3，容量没变，实际相当于做了**arr[3]=100**的操作。

② s是一个切片，x和y是s的切片，所以它们和s共享底层数组。执行了**x[1]=99**相当于执行了**s[2]=99**。

③ s是一个切片并且当前**cap > len**，在调用**modify(s)**的时候，相当于把**s**复制了一份，给到了变量**v**，在调用了**v = append(v, 999)**后，**v**的长度变成了4，但**s**并没有变，所以**len(s)**还是3,看不到**999**（999实际在s[3]的位置）。调用了**v[0]=111**后，相当于执行了**s[0]=111**, 所以最后print s的时候能看得见

**实际跑完的验证结果（对了/错在哪）：


- 反直觉：
- 踩的坑：
- 没搞懂：
1. 关于切片的数据结构实质上是**[ 指向底层数组的指针 | len | cap ]**，问题是，有了指向底层数组的指针，有了len，那它怎么知道起始位是多少呢？

---

## D4 · struct、方法、组合（2026-08-17）

讲解见 `lessons/D4.md`，练习在 `internal/payroll/`。
先跑 `go run ./cmd/embeddemo`，第 ② 段是今天的题眼。

**小题 A · 接收者与方法集谜题**

三段代码，**先不要跑**，用脑子推。

```go
// ① 值接收者能不能改到原对象
type Counter struct{ N int }
func (c Counter) Inc()  { c.N++ }
func (c *Counter) Add() { c.N++ }

func a() {
	c := Counter{}
	c.Inc()
	c.Add()
	p := &Counter{}
	p.Inc()
	p.Add()
	fmt.Println("c.N =", c.N, " p.N =", p.N)
}

// ② 下面四行，哪些编译不过？
type Speaker interface{ Speak() string }
type Cat struct{}
func (c *Cat) Speak() string { return "喵" }

func b() {
	var s1 Speaker = &Cat{}     // (1)
	var s2 Speaker = Cat{}      // (2)
	c := Cat{}
	fmt.Println(c.Speak())      // (3)
	m := map[string]Cat{"a": {}}
	fmt.Println(m["a"].Speak()) // (4)
}

// ③ 嵌入的方法里，this 指的是谁
type Base struct{ Name string }
func (b Base) Hello() string { return "Hello, " + b.Who() }
func (b Base) Who() string   { return "Base" }

type Sub struct{ Base }
func (s Sub) Who() string { return "Sub" }

func c() {
	s := Sub{Base{Name: "x"}}
	fmt.Println(s.Who())
	fmt.Println(s.Hello())
}
```

我的答案：

```
① c.N = 1       p.N = 2

② 编译不过的是：( (3)和(4)  )

③ s.Who()   = Sub
   s.Hello() = Hello, Base
```

为什么（③ 请说清楚「为什么 Hello 里调不到 Sub.Who」）：
① 
**c**是一个结构体，**Inc**方法的接受者是**Counter**，所以在调用**c.Inc()**的时候，相当于
把**c**复制了一份丢给了**Inc**，在它内部对那个拷贝后的结构体做修改不影响**c**。
**Add**方法的接受者是**Counter**类型的指针，所以在调用**c.Add()**的时候，相当于把**c**的地址丢给了**Add**，在它内部对那个指针指向的结构体做修改，所以**c.N**会增加1，所以最后是1

**p**本身是**Counter**结构体实例的指针，所以在调**p.Inc()**和**p.Add()**都会改到这个实例本身，所以最后是2

②
第(3)编译不过，是因为**Speak**这个方法是绑定在指向**Cat**结构体实例的指针上的，而**c**是这个结构体的一个实例本身。
第(4)编译不过，是因为从map通过**key**拿出来的东西是不可寻址的，因为在map扩容时，内部的数据会被重新分配内存地址。

③
打印**s.Who()**的时候，因为286行那里给Sub绑定了一个**Who**函数，相当于覆盖了**Base**里的**Who**，所以其实调用的是这个**新的Who**。
打印**s.Hello()**的时候，因为**Hello**方法没有被覆盖，go一路找上去，在嵌入的**Base**里找到了**Hello**方法，调用它，它内部调的**Who**函数只能是**Base**自己定义的，因为它不可能知道自己被嵌入到什么里面去了，所以最终输出**Hello, Base**


实际跑完的验证结果（对了/错在哪）：

**① ❌ 错了：`p.N` 是 1，不是 2。**（实测输出 `c.N = 1  p.N = 1`）

`c` 那半边的推理是对的。错在 `p`：我以为「手里是指针 → 调什么方法都能改到原对象」。
不成立。**「有没有拷贝」只由接收者类型决定，和调用者手里拿的是值还是指针无关。**

`p.Inc()` 会被编译器展开成 `(*p).Inc()`，运行时两步：
1. 解引用 `*p`，顺着地址找到那个真正的 `Counter` 实例；
2. 把它整个拷贝一份传给 `Inc`。方法里的 `c` 是全新变量，`c.N++` 改的是副本。

打地址验证过，两次 `p.Inc()` 里的 `&c` 是两个不同地址（每次都新拷一份），
而 `p.Add()` 里的 `c` 就是 `main` 里的 `p` 本身：

```
main 里     p = 0x...4020
  Inc  里 &c = 0x...4028     ← 拷贝
  Inc  里 &c = 0x...4030     ← 又一份新拷贝
  Add  里  c = 0x...4020     ← 就是原件
```

四种组合，唯一决定拷贝的是**列**，不是行：

| 手里是 | 调值接收者方法 | 调指针接收者方法 |
|---|---|---|
| 值 `c` | 直接传 → **拷贝** | `(&c)` → **取地址，无拷贝**（要求可寻址） |
| 指针 `p` | `(*p)` → **解引用 + 拷贝** | 直接传 → **无拷贝** |

自动取地址 / 自动解引用只是帮我把类型对上，从不改变这件事。

推论 1（nil 接收者）：
```go
var p *Counter   // nil
p.Add()   // ✅ 不 panic —— 只是把 nil 传进去，没碰字段就没事
p.Inc()   // ❌ panic —— 必须先解引用才能拷贝，nil 解不出东西
```
推论 2（性能）：这个拷贝的成本是**整个 struct 的大小**。大 struct 在热路径上用值接收者
就是白白的内存拷贝——这才是「struct 大就用指针接收者」的真正理由。

**② ❌ 答反了：编译不过的是 (2) 和 (4)，不是 (3) 和 (4)。**

- **(3) 能编译。** `c` 是可寻址的局部变量，编译器改写成 `(&c).Speak()`。
  我的理由「方法绑在指针上，c 是实例本身」正是要被拆掉的直觉。
- **(2) 编译不过**，这才是指针接收者真正会咬人的地方：
  ```
  cannot use Cat{} as Speaker value: Cat does not implement Speaker
      (method Speak has pointer receiver)
  ```
- **(4) 我答对了，理由也对**：map 元素不可寻址。

自查：如果我的模型（「值上不能调指针方法」）成立，(2) 也该被我标出来，但我漏了。
说明我当时把 (3)(4) 当成同一类问题，没看出 (3) 不挂和 (4) 挂其实是**同一条规则的两面**
——能不能取到地址。三行并排：

| 写法 | 结果 | 原因 |
|---|---|---|
| `c.Speak()`，`c` 是变量 | ✅ | 可寻址 → `(&c).Speak()` |
| `m["a"].Speak()` | ❌ | map 元素不可寻址 |
| `var s Speaker = Cat{}` | ❌ | **接口赋值不走这条路**：方法集是硬规则，`Cat` 的方法集里没有 `Speak` |

第三行是独立的规则（方法集，D4.md §3 那张表），别和前两行混着记：
**可寻址 → 编译器帮忙；接口赋值 → 只认方法集，不帮忙。**

**③ ✅ 全对**，理由也精准（「它不可能知道自己被嵌入到什么里面去了」）。

**小题 B · 设计题（写在下面，不用写代码）**

`Contractor` 没有嵌入 `Employee`，它凭什么能进 `[]Payer`？
如果这套东西用 Java 继承来做，你会怎么安排类层次？会遇到什么别扭的地方？
回答：因为`Contractor`也有**MonthlyPay**和**Label**这2个方法，符合**Payer**这个接口的定义。如果用java来做，我也可以让`Contractor`继承`Employee`，然后构造函数里面需要输入**Hours**和**HourlyRate**，在**MonthlyPay**方法里面，我需要根据**Hours**和**HourlyRate**来计算薪酬总额。

补：前半对，后半只说了「怎么绕」，没答出问的「别扭在哪」。
`Contractor extends Employee` 之后的四处别扭：
1. 背上一个用不上的 `baseSalary`（是 0，但每个读代码的人都要停下来想为什么）；
2. 抽象方法 `bonus()` 对它毫无意义，只能 `return 0` 或抛 `UnsupportedOperationException`（LSP 违反的经典味道）；
3. 模板方法 `calculatePay() = baseSalary + bonus()` 对它是错的，子类第一件事就是掀翻基类骨架——这时候继承已经零收益，只剩耦合；
4. 真要做干净，得在 `Employee` 之上再抽一个 `Payable` 接口——**Java 最后也会走到接口，只是要先动整个类层次。**

所以 Go 的省事不在于「不用写接口」，而在于**接口能后加**：`Payer` 是后定义的，
`Contractor` 一行没改就满足了。Java 里做不到，除非能改 `Contractor` 的源码去 `implements`。


- 反直觉：
- 踩的坑：
- 没搞懂：

---

## D5 · interface 与泛型（2026-08-17）

讲解见 `lessons/D5.md`，练习在 `internal/store/`（主）和 `internal/genx/`（副）。
先跑 `go run ./cmd/ifacedemo`，第 ④ 段是题眼。

**小题 A · 接口值与泛型约束谜题**

四段代码，**先不要跑**，用脑子推。

```go
// ① nil 接口
type MyErr struct{}
func (e *MyErr) Error() string { return "boom" }

func f() error { var e *MyErr; return e }
func g() error { return nil }

func a() {
	fmt.Println(f() == nil, g() == nil)
	fmt.Println(f())
}

// ② type switch 的匹配顺序
func kind(v any) string {
	switch v.(type) {
	case error:
		return "error"
	case *MyErr:
		return "*MyErr"
	default:
		return "other"
	}
}

func b() {
	fmt.Println(kind(&MyErr{}))
	fmt.Println(kind(nil))
	fmt.Println(kind(42))
}

// ③ 约束里的波浪号
type MyInt int
func SumA[T int | float64](s []T) T  { ... }
func SumB[T ~int | ~float64](s []T) T { ... }

func c() {
	SumA([]int{1, 2})     // (1)
	SumA([]MyInt{1, 2})   // (2)
	SumB([]MyInt{1, 2})   // (3)
}

// ④ comparable 能挡住什么
func Uniq[T comparable](s []T) int { m := map[T]struct{}{}; for _, v := range s { m[v] = struct{}{} }; return len(m) }

func d() {
	fmt.Println(Uniq([]any{1, "a", 1}))   // (1)
	fmt.Println(Uniq([]any{[]int{1}}))    // (2)
}
```

我的答案：

```
① 第一行 = false, true
   第二行 = <*MyErr, nil>

② kind(&MyErr{}) =  *MyErr          kind(nil) =    other        kind(42) = other

③ 编译不过的是：(  (2)      )

④ (1) = 2               (2) = 1
```

为什么（① 请说清楚「为什么 f() 不等于 nil，但打印出来是 <nil>」）：
①
**f**里面定义了一个指向**MyErr**类型的指针，但还当前还没有指向任何实例，所以它本身是nil。
在**return e**的时候，因为**f**本身定义的返回类型是**error**，是个接口，所以go会在这里做一次
装箱操作，实际返回<*MyErr, nil>，所以**f() != nil**。**g**因为写死的就是返回**nil**，所以**g() == nil**。

②
**kind(&MyErr{})**里面的参数实际类型是指向**MyErr**的指针，所以走到了**case *MyErr**里。
nil和42不在任何一个case里，所以走了default分支

③
**SumA**定义的范型参数是严格的**int**或者**float64**，所以传入**MyInt**类型会出错。
**SumB**定义的范型参数的时候用了波浪线～, 说明可以接受任何底层实际类型是**int**或者**float64**的东西，所以可以传入**MyInt**类型。

④
第一个**Uniq**调用，传进去的是一个值是**1, a, 1**的数组，所以去重后长度是2。
第二个**Uniq**调用，传进去的是一个值是[1]的数组，值本身是一个数组，但只有这一个值，所以长度是1。


实际跑完的验证结果（对了/错在哪）：

**① 第一行 ✅ `false, true`**，解释也完全对（装箱、静态类型都说到了）。

**① 第二行 ❌**：我答的 `<*MyErr, nil>` 是接口**内部的状态**，不是 `fmt.Println` **打印出来的东西**。
实测打印的是 **`boom`**。

原因（Claude 第一版讲义在这里也写错了，一起纠正）：**`fmt` 遇到 error 会调用它的
`Error()` 方法**，打印结果取决于这个方法碰没碰接收者：

```
type A struct{...}; func (e *A) Error() string { return "boom" }                    // 不碰接收者
type B struct{...}; func (e *B) Error() string { return fmt.Sprintf(..., e.Code) }  // 解引用了

var errA error = (*A)(nil)  →  fmt.Println(errA) 打出  boom    ← 方法正常返回了
var errB error = (*B)(nil)  →  fmt.Println(errB) 打出  <nil>   ← 方法 panic，被 fmt 兜住了
```

那个 `<nil>` 是 **fmt 内部的兜底**（捕获 panic，发现实参是 nil 指针），**不表示「值是 nil」**。
所以日志里可能看到 `boom` / `<nil>` / `%!v(PANIC=...)`，**三种都不告诉你这个 error 其实非 nil**。
**可信的只有 `%T` 和 `== nil`。**

**② `kind(&MyErr{})` ❌ 是 `error`，不是 `*MyErr`。**（`kind(nil)`、`kind(42)` 都对）

```
kind(&MyErr{}) = error    kind(nil) = other    kind(42) = other
```

`*MyErr` 实现了 `error`，而 `case error` 排在 `case *MyErr` **前面** ——
**type switch 从上往下匹配，第一个命中就返回**，后面那个 `case *MyErr` 是永远走不到的死代码。

⭐ 规则：**case 越具体越靠前，接口 case 垫底。**

`kind(nil)` 答对了，理由也对：nil 接口不匹配任何类型 case（连 `case error` 都不匹配），
只有独立的 `case nil` 能接住它。

**③ ✅ 全对**，`~` 的解释准确。

**④ (1) ✅ = 2。(2) ❌** —— 我空着没填，但解释里写的「长度是 1」是错的。实际是**运行时 panic**：

```
panic: runtime error: hash of unhashable type []int
```

它**编译得过**（Go 1.20 起 `any` 满足 `comparable`），但 `[]int` 不可哈希，
往 map 里塞的瞬间就炸。**`comparable` 挡不住装了 slice 的 `any`：编译期放行，运行时才炸。**


**小题 B · 设计题（写在下面，不用写代码）**

1. `store` 包里我把接口切成了 `Getter` / `Putter` 两个单方法接口，而不是直接写一个
   有 `Get`/`Put`/`Delete`/`List` 的 `Store`。**切小了到底换来了什么具体的好处？**
   （提示：看 `CopyAll` 的签名，和 `store_test.go` 里那几个测试替身。）

2. 下面四个需求，各自该用泛型还是接口？为什么？
   - a. 写一个函数，取出任意 map 的所有 key
   - b. 写一个函数，把「用户」「订单」「商品」都渲染成一行日志
   - c. 写一个 LRU 缓存，键和值的类型由调用方决定
   - d. 写一个函数，对「能算出金额的东西」求和（回想 D4 的 `Payer`）

回答：
1. 接口切小了可以更容易适配到某个结构体上，比如有个**只读**的Store它只能**Get**和**List**，如果是一个定义了`Get`/`Put`/`Delete`/`List`的大接口，那么如果有个方法只需要读Store，就没办法传那个**只读Store**了。

2. 
a用泛型，因为不需要区分实际是什么类型。
b用接口，因为需要区分实际是什么类型 - 日志里描述「用户」「订单」「商品」的措辞会有所不同。
c用泛型，因为不需要区分实际是什么类型。
d用泛型，因为不需要区分实际是什么类型。

**批改：**

**第 1 问 ✅ 核心答对**（大接口会让只读实现传不进去），但还漏了两笔更具体的收益：

1. **签名即文档，且编译器强制**：`CopyAll(dst Putter, src Getter, ...)` 一眼看出方向 ——
   dst 只被写、src 只被读。**在实现里手滑写 `src.Put(...)` 是编译错误。**
   两个参数都是大 `Store` 的话，把 dst/src 传反了照样编译通过、照样跑，然后把数据洗反。
2. **测试替身的成本**：`getterOnly` 写 1 个方法就能用；大接口要写 4 个，其中 3 个是
   `panic("not implemented")`。更糟的是**接口以后加方法时，所有假实现全部编译失败**。
   （Java 里 `implements` 大接口时 IDE 生成一屏 `UnsupportedOperationException`
   ——那一屏就是接口太大的账单。）

**第 2 问：a ✅ b ✅ c ✅ d ❌ —— d 应该用【接口】。**

「能算出金额」是一种**行为**，每个类型算法不同（Manager 是底薪+提成，Contractor 是工时×时薪）。
就是 D4 那道 `Payer`。

我的判据「需不需要区分实际类型」在 d 上失灵了，因为从调用方看确实"不需要区分"。换个说法：

> **泛型：同一段代码，作用于不同类型**（代码相同，类型不同）
> **接口：不同的代码，通过同一个名字被调用**（名字相同，代码不同）

对照我自己写的 `Sum[T Number]`：`total += v` 对每个 T 是**同一个加法**，编译器生成同一段逻辑。
而 `p.MonthlyPay()` 源码上看着统一，运行时**派发到不同实现** —— 这就是多态，只有接口给得了。

**硬证据**：泛型版本写得出来（`func TotalGeneric[T Payer](ps []T) int` 合法编译），但用不了：

```
接口版，混合类型:      42500   ✅  []Payer{Manager{...}, Contractor{...}}
泛型版，只有 Manager:  10500   ✅  []Manager{...}
泛型版，混合类型:      编译错误 ❌
    in call to TotalGeneric, T (type any) does not satisfy Payer (missing method MonthlyPay)
```

`[]T` 是**同质**的（所有元素必须同一个类型），`[]Payer` 是**异质**的。
⭐ **要「一堆不同类型放在一起」，就只能是接口。**
泛型给的是「同一份代码套用到不同类型上」，不是「不同类型混在一起」。

补 c 的细节：`Cache[K, V]` 用泛型对（增删查代码对所有 K/V 一样）；但**可插拔的淘汰策略**
（LRU/LFU/TTL）那部分得是接口。**一个类型里两者常常并存 —— 泛型管数据形状，接口管可变行为。**


- 反直觉：
1. store.go的Put方法里，需要手动拷贝一下Tags，以防在其他地方修改了Tags里的内容导致和已经调用Put那一瞬间的数据不一样。
- 踩的坑：
1. 同“反直觉#1”
- 没搞懂：

---

## D6 · 测试与工程规范（2026-08-17）

讲解见 `lessons/D6.md`，练习在 `internal/cache/`。
今天没有新语法，题眼是 §5：**覆盖率 100% 说明不了任何事**。

先感受一下：

```bash
go test -cover ./internal/cache/
#  ok  ...  coverage: 100.0% of statements     ← 全绿，100% 覆盖，但有 3 个 bug
```

**小题 A · 工程规范推理题**

五问，**先不要查文档、不要跑**，凭理解答。

```
① internal/ 的规则
   下面哪些 import 会【编译失败】？模块名是 github.com/me/proj
   (1) github.com/me/proj/cmd/app        里 import github.com/me/proj/internal/db
   (2) github.com/me/proj/internal/db    里 import github.com/me/proj/internal/log
   (3) github.com/other/thing            里 import github.com/me/proj/internal/db
   (4) github.com/me/proj/internal/a/b   里 import github.com/me/proj/pkg/c

② MVS 选哪个版本
   你的模块要求 lib v1.2.0
   依赖 A     要求 lib v1.5.0
   依赖 B     要求 lib v1.3.0
   go build 会用 lib 的哪个版本？如果 lib 此刻最新版是 v1.9.0，答案会变吗？

③ 下面这个测试的覆盖率是多少？它能发现 Add 的 bug 吗？
   func Add(a, b int) int { return a - b }        // 故意写错
   func TestAdd(t *testing.T) { Add(1, 2) }

④ 这段基准测试的结果可信吗？为什么？
   func BenchmarkSum(b *testing.B) {
       data := make([]int, 100000)
       for i := 0; i < b.N; i++ {
           Sum(data)
       }
   }

⑤ 这两个测试文件有什么区别？各自能做什么、不能做什么？
   // a_test.go
   package cache
   // b_test.go
   package cache_test
```

我的答案：

```
① 编译失败的是：( (3)       )

② 用的版本 =     v1.5.0       ；最新版是 v1.9.0 时答案(不会)变。因为go会在满足所有约束的版本集合里取最小。go本身也不会去查最新版。

③ 覆盖率 =  100%      ；不能发现 bug，因为“覆盖率100%”只说明代码的每一行都被跑到了，但是测试里并没有对结果做验证，所以不管实际代码实现逻辑如何（只要不panaic,不fatal error），总归是过的。

④ (不可信)，因为for循环应该改成b.loop，让go自己决定要跑多少轮才能达成一个稳定的数字

⑤
a_test.go是白盒测试，它能摸到被测代码里私有的东西。
b_test.go是黑盒测试，它只能测试被测代码里公开暴露出来的东西。
黑盒能强迫你从使用方视角设计 API（白盒永远发现不了 API 缺东西）；黑盒不会循环导入
```

为什么（③④ 请说清楚）：
①
(3)github.com/other/thing 是 外部模块 ，完全不在 github.com/me/proj 这个树里， 不能 导入别人的 internal 。这是 internal 机制的核心目的—— 防止外部依赖实现细节 。

②
题目里已经回答原因了

③
题目里已经回答原因了

④
题目里已经回答原因了

⑤
题目里已经回答原因了


实际跑完的验证结果（对了/错在哪）：

**① ✅ 只有 (3)**，理由准确。

补一条我没说全的：**(4) 之所以合法，是因为「从 internal 里往外 import」永远没限制**。
`internal` 只管「谁能进来」，不管「里面的人能去哪」。

**② ✅ v1.5.0；不会变。** 但措辞要改准：

- ❌ 我写的「选这个包里面最小版本」
- ✅ 准确说法：**在「满足所有约束的版本集合」里取最小**

三个数字（v1.2.0 / v1.3.0 / v1.5.0）不是候选版本，是三条**下界**。
满足全部下界的集合是 `{v1.5.0, v1.6.0, ... , v1.9.0, ...}`，Go 取其中最小的 v1.5.0；
npm/Maven/Cargo 取其中最大的 v1.9.0。**「最小」是相对于别的包管理器说的。**

更关键的一点我没答出来：**Go 根本不去查最新版是多少**，所以 v1.9.0 存不存在对结果毫无影响。
这才是它和 npm/Maven 的根本分歧（构建可复现，不需要 lockfile）。

**③ ✅ 完全正确。**「只要不 panic 总归是过的」抓住了要害。

**④ ⚠️ 结论对（不可信），但理由错了。**

> 我写的：因为 for 循环应该改成 b.Loop，让 go 自己决定要跑多少轮

**老写法 `for i := 0; i < b.N; i++` 的轮数也是 Go 决定的** —— `b.N` 就是框架填进去的。
所以「让 go 决定轮数」根本不是两者的区别，我把 `b.Loop()` 的好处记错了。

那段代码不可信的真正原因有两个：

```go
func BenchmarkSum(b *testing.B) {
	data := make([]int, 100000)        // ① 这行的耗时【被计入】了
	for i := 0; i < b.N; i++ {
		Sum(data)                       // ② 结果被丢弃 → 可能被编译器整个优化掉
	}
}
```

- **①** 老写法里循环之前的准备工作会算进计时，要手动 `b.ResetTimer()`；`b.Loop()` 自动排除。
- **②** 这条才致命 —— `Sum(data)` 的返回值没人要，编译器能证明这次调用无副作用，
  于是整个删掉，你测出一个假的极小值。修法是 **sink**（包级变量接住结果）。

⭐ 记住：**`b.Loop()` 的两个好处是「自动排除 setup 计时」和「保证调用不被删掉」，
不是「决定轮数」。** 而且它也**挡不住逃逸分析**（见 D6 §6），
所以 sink 该加还是要加 —— 我在 `genx_bench_test.go` 里就是这么做的。

**⑤ ✅ 对。** 可以再补两条黑盒的好处：
- **强迫你从使用方视角设计 API** —— 白盒永远发现不了 API 缺东西（它可以伸手进去拿私有成员）
- **不会循环导入** —— 包 A 的测试要用包 B、而 B 又导入 A 时，白盒会编译失败，黑盒不会


**小题 B · 主练习的交付说明（写在下面）**

三个 bug 各自：**在生产环境会造成什么后果？** 用一句话说，要具体到「用户会看到什么」。

- bug 1（时间逻辑）：过期的数据用Get函数还能被取到。可能导致的用户场景：比如用户密码过期了，还能被验证正确。
- bug 2（统计口径）：过期的数据用Len函数还能被统计到。可能导致的用户场景：缓存监控页面显示的使用率错误地偏高。
- bug 3（并发）：
a. 并发环境下，Get函数里对hit进行了+1的操作，会导致两个Get并发调用互相override，假设初始值是0，两次Get调用后理应是2，实际却是1 - 每个都是0+1后写回去。
b. 出错：“fatal error: concurrent map writes”。并发写 map 会让整个进程直接崩溃，而且是 fatal error 不是 panic，recover() 拦不住。题目问「用户会看到什么」，答案是 502 / 服务挂了，不是「统计数字少了一点」。

另外回答：**为什么 100% 的覆盖率一个都没抓到？** 三个 bug 漏网的原因一样吗？
因为测试代码只是走完了被测试代码的所有分支，但是并没有对约定的处理逻辑做完整的验证。
三个bug的原因不一样：bug 1、2：代码走到了，但断言不够 → 加强断言就能抓到
- bug 3：再强的断言也没用。单 goroutine 跑一万遍也不会表现出竞态，它需要的是不同的执行环境（多 goroutine + -race）


- 反直觉：
- 踩的坑：
1. 写测试的时候，对比函数注释里的约定/规范，还是有漏了的逻辑没测到
2. TestBugs_LenWithExpiredData一开始是「1 个过期 + 1 个新鲜」，所以不管是即使代码有bug,返回的是“过期”的那一个，结果也是1，测不出问题。
**教训：构造测试数据时，要让期望值和各种错误实现的输出都不一样。 等值、对称、0/1 这些取值特别容易「碰巧对」**
3. Len 里的 我一开始加了这段逻辑
```go
if c.ttl <= 0 { return 0 } 
```
撞对了负 TTL 的答案, 但那一版里我在**if e.expiresAt.After(c.now())**前面取反了，写成了**if !e.expiresAt.After(c.now())**，虽然负 TTL 的测试通过了，但其实有问题，会导致其他case失败。
4. TestCopyAllWithError 第一版把覆盖率刷到了 100%，但变异测试 0/4——%w 改 %v、去掉 id、不包装、copied 返回 0，一个都抓不到。
**危险之处在于：覆盖率报告会告诉你这里已经测过了。 一行代码承诺了几件事，就该有几条断言。**
- 没搞懂：

---

## D7 · 周综合项目：日志分析 CLI（2026-08-20）

讲解与项目说明见 `lessons/D7.md`，代码在 `internal/logx/` + `cmd/loganalyze/`。
本周没有新知识点，把 D1–D6 全用一遍。

**设计问答（讲义 §1.4 留的五问，review 时会逐条问）**

1. `Report` 能不能做到零值可用（D4 §1）？如果能，为什么我还是给了你一个 `NewReport`？
   回答：不能，因为里面有map类型的字段，map零值不可写入数据

2. `Percentile` 用最近秩法，文档里约定了「空数据返回 0 / p<=0 取最小 / p>=1 取最大」。
   你后来发现那两个边界特判在代码里是冗余的 —— 那**文档里还要不要写这两条约定**？为什么？
   回答：要的，文档约定的是对外的行为准则，我的代码是内部实现，从实现上多余的，但是这个行为还是有，因为被那个算法覆盖了掉了。

3. `ParseLine` 失败时返回零值 `Entry`。换成「填一半」（比如时间解析成功了就先填上）
   会有什么问题？调用方可能怎么误用？
   回答：如果只返回部分数据，比如你说的只返回了时间，很有可能造成调用方调用其他字段时得到非预期的值，大概率是那个类型的零值。

4. `Formatter` 该不该加一个 `Name() string` 方法，让 CLI 自动列出可用格式？
   加了有什么代价？（提示：D5 §2「接口越小，能塞进去的类型越多」）
   回答：加了代价是，每个实现都要实现 `Name` 方法，而这并不是一个Formatter本身必须要实现的逻辑 - 它不用告诉别人自己叫什么，只要能完成自己的format逻辑就行。如果只是为了让CLI 自动列出可用格式可以简单地定义一个可导出的常量来表示。

5. 耗时字段用 `time.Duration` 而不是 `int`，好处是什么？
   但 JSON 输出时又转回了毫秒整数 —— 这个「进来转、出去再转」值不值？
   回答：time.Duration是go内置的，它用了int64类型，所以可以表示任意大的时间间隔。而int类型只能表示0到2147483647，所以不能表示任意大的时间间隔。更重要的是，time.Duration还提供了一系列时间操作函数和打印函数


**本周复盘（D1–D7）**

- 反直觉：
- 踩的坑：
  1. 做Benchmark的时候，被循环测试代码需要从io.Reader读数据。我一开始用**os.Open**当作Reader传进去，导致在第二轮开始后，这个file reader其实已经读完了，被测的代码变得没有意义 - 其实等于没被测到，值循环了一次。
  2. 写Example测试的时候，如果期望的结果是多行的字符串且中间有空白行，那空白行前面也要加上注释标签 **//** ，否则会报错
- 没搞懂：

---

## D8 · goroutine 与 channel（2026-08-20）

讲解见 `lessons/D8.md`，练习在 `internal/crawl/`。
先跑 `go run ./cmd/chandemo`，第 ⑦ 段是题眼（goroutine 泄漏）。

**小题 A · channel 语义谜题**

五段代码，**先不要跑**，用脑子推。

```go
// ① 无缓冲 channel 的时序
func a() {
	ch := make(chan int)
	go func() {
		fmt.Println("A")
		ch <- 1
		fmt.Println("B")
	}()
	time.Sleep(50 * time.Millisecond)
	fmt.Println("C")
	<-ch
	time.Sleep(50 * time.Millisecond)
}
// 打印顺序是？

// ② 关闭之后还能读到什么
func b() {
	ch := make(chan int, 2)
	ch <- 1
	close(ch)
	v1, ok1 := <-ch
	v2, ok2 := <-ch
	fmt.Println(v1, ok1, v2, ok2)
}

// ③ 下面每一行【单独】看会发生什么？（panic / 死锁 / 无事发生）
//
// ⚠️ 读法：假设每行都在「它上面那些行建立的状态」下执行，
//    并且前面的 panic 不中断后面的行。也就是说 (2)(3) 面对的是一个【已关闭】的 ch。
func c() {
	ch := make(chan int)
	close(ch)            // 建立状态：ch 已关闭
	close(ch)            // (1)
	ch <- 1              // (2)
	<-ch                 // (3)

	var nilCh chan int   // 建立状态：nilCh 是 nil
	nilCh <- 1           // (4)
	close(nilCh)         // (5)
}

// ④ WaitGroup 放错位置
func d() {
	jobs := make(chan int)
	results := make(chan int)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			for j := range jobs { results <- j * 2 }
		})
	}
	go func() { for i := range 5 { jobs <- i }; close(jobs) }()

	wg.Wait()            // ⚠️ 注意这一行的位置
	close(results)
	for r := range results { fmt.Println(r) }
}
// 会发生什么？

// ⑤ 这段会不会泄漏 goroutine？
func e() <-chan int {
	ch := make(chan int)
	go func() {
		for i := range 3 { ch <- i }
		close(ch)
	}()
	return ch
}
func caller() {
	for v := range e() {
		if v == 1 { return }   // ⚠️ 提前 return
	}
}
```

我的答案：

```
① 打印顺序 = A C B

② 输出 = 1 true 0 false

③ panic: ( 1  2  5 )   死锁: ( 4 )   无事发生: ( 3 )

④ 这段代码死锁

⑤ 会泄漏，因为ch是无缓冲的，所以在go routine里往ch写值的时候就堵塞等人来读。在caller函数里，for循环确实开始读ch了，但是因为拿到第一个值就return导致整个主流程结束执行，这时候go routine就卡在往ch里写第二个值，永远不会结束。
```

为什么（①④⑤ 请说清楚）：
①
首先，在844行**time.Sleep(50 * time.Millisecond)**睡了50ms，所以轮到go routine里的A被打印出来。然后go routine往ch里放了一个值，因为这个ch是无缓冲的，所以go routine这时候要停下来等人从ch里读这个值。50ms过后，主流程把C打印出，然后从ch里读取这个值，所以go routine继续执行，打印出B。

②
ch在close前先被写入了一个**1**，因为它是有缓冲的，缓冲长度是2，所以放了一个值之后不会堵塞等别人读取。然后ch被close。第一次从ch读取的时候，因为里面有还未被读取的值，所以v1是1, ok1是true(虽然关闭但还有值)。第二次读取的时候，因为ch里面已经没有东西而且已经被关闭，所以v2度到的是int对应的零值，也就是0， ok2是false - ch已关闭且没有未被读取的值。

③
(1) panic：close 一个已经被 close 的 ch。
(2) panic：往一个已经被 close 的 ch 里写数据。注意它是【立刻炸】，不是阻塞等人读 —— 
    channel 一旦关闭，「无缓冲要等配对」这条规则就不适用了。
(3) 无事发生：从一个已关闭的 ch 接收，立刻返回 (零值, false)，既不 panic 也不阻塞。
(4) 死锁：往 nil channel 写数据永远阻塞。
(5) panic：close 一个 nil channel。

⭐ 这题的核心是一张 2×2 表 —— 关闭与否，把「发送/接收」的命运完全换了一套：

| ch 的状态          | 发送 ch <- v | 接收 <-ch          |
|--------------------|--------------|--------------------|
| 已关闭             | **panic**    | **立刻返回 (0,false)** ✅ 安全 |
| 未关闭、无人配对   | 阻塞 → 死锁  | 阻塞 → 死锁        |

一句话：**关闭之后，只有接收是安全的**（D8 §3）。这也是「只有发送方能关闭」的理由 ——
接收方永远不会因为关闭而 panic，发送方会。

【我第一次答错的地方】我把 (3) 判成了死锁，理由是「(2) 卡住了所以走不到 (3)」。
两个问题：
  a. 前提混用了 —— 说 (2) panic 需要「ch 已关闭」，说 (3) 死锁需要「ch 未关闭」，
     同一道题里对同一个 ch 用了两种状态。
  b. 就算按「ch 未关闭」算，死锁的【现场】也是 (2)，(3) 只是【不可达】。
     「这一行会死锁」和「因为前面死锁了，这一行执行不到」是两回事 ——
     SIGQUIT dump 出来的栈里，行号指的是现场那一行。

④wg.Wait()和下一句close应该放在一个独立的go routine里。wg.Wait这一句会阻塞主流程。然后在wg.Go生成的go routine往results这个channel写数据的时候会堵塞等人来读（因为它是无缓冲的），然而读取results这个channel的代码是在主流程里且在wg.Wait后面，导致实际不会被执行，那workder的go routine也就卡着，造成这段代码死锁。

⑤
已经在上面回答。


实际跑完的验证结果（对了/错在哪）：

**① ✅ `A C B`** —— 「go routine 要停下来等人从 ch 里读」抓住了无缓冲的本质。

**② ✅ `1 true 0 false`** —— 解释也准确。

**③ 见上面改写过的答案。**（题目原本有歧义，Claude 改清楚了；我的语义理解是对的，
错在把两种前提混用，以及把「死锁现场」和「不可达」当成一回事。）

**④ ✅ 死锁，而且解释是五道里写得最好的一段** —— 完整说出了那条环：
主流程卡在 Wait → worker 卡在发结果 → 读 results 的代码在 Wait 后面永远执行不到。
这就是 QA #15 的等待图。

**⑤ ✅ 会泄漏**，「卡在往 ch 里写第二个值」定位精确。

---

**小题 B 批改**

**1 · 我答「不知道」，答案是：**

先纠正前提 —— **`Future.cancel(true)` 也杀不掉线程**。它调的是 `Thread.interrupt()`，
**只设一个标志位**；目标线程必须自己检查 `isInterrupted()`，或者正好阻塞在可中断的
调用里（sleep/wait/join）才会响应。跑死循环又不检查标志的线程，`cancel(true)` 拿它没办法。

**所以 Java 的取消也是协作式的**，和 Go 的 context 是同一类东西。

真正的「杀死」是 `Thread.stop()` —— **Java 1.2 就废弃了**，文档给的理由是：
它会释放该线程持有的**所有** monitor，让被这些锁保护的对象处于**不一致状态**，且毫无征兆。

```go
mu.Lock()
m["a"] = 1
// ← 在这里被杀死
m["b"] = 2      // 永远不会执行
mu.Unlock()     // defer 也不会执行 → 锁永远不释放
```

**Java 试过，失败了，废弃了；Go 直接不提供这个坑。** 唯一安全的取消是
「请求 + 对方在安全点自己退出」——这就是 context（D10）。

**2 · ⚠️ 原理对，但答的是另一个对象。**

我说的是 `FetchAll` 里的**结果切片**（每格一个写入方）—— 那个理解正确。
但题目问的是 `Crawl` 里的 `seen` map，它不需要锁的理由不一样：

> **并发发生在 `FetchAll` 内部，而 `seen` 只在 `FetchAll` 返回【之后】才被读写。**
> 那时所有 worker 都已退出，只剩 `Crawl` 这一条 goroutine 在跑。

不是「每格一个主人」，而是「整张表只有一个主人，且那个时刻没有别人在跑」。

什么时候需要加锁：**如果改成不分层的 worker pool，worker 自己发现链接、自己判重、
自己入队** —— 那时 N 个 worker 同时读写 `seen`，必须加锁（或者把链接发回给一条
专门的协调 goroutine，那又不需要锁了）。

**3 · ✅ 好答案，补一个角度。**

「测试随机失败」和「调用方无法对应输入输出」都对。但**「实现会更简单还是更复杂」
我答成了「更麻烦」—— 实际上无序实现更简单**（不用传下标、不用回填数组，
worker 直接往 results 发）。而且对某些调用方无序更好：想显示进度条、
想「拿到第一个成功的就停」，无序才做得到。

**所以这是取舍，不是单向的好坏。** 批量抓完再处理 → 有序；流式处理 → `<-chan Result` 更好。
**API 形状应该由使用场景决定。**


**小题 B · 设计题（写在下面，不用写代码）**

1. `FetchWithTimeout` 超时之后，那次 `Fetch` **还在继续跑**。我们只是「不再等它」。
   这和 Java 的 `Future.cancel(true)` 有什么区别？为什么 Go 不提供「杀死 goroutine」？
   回答：不知道。你说D10会讲的context能做到取消，但我不知道go为什么这样设计。

2. D8 §9 说「不要用共享内存来通信」不等于「不许用 mutex」。
   你在 `Crawl` 里的去重表没加锁 —— 说清楚**为什么不需要**。
   什么情况下它就需要加锁了？
   回答：在`FetchAll`方法里因为结果集切片里的每一个数据都只有一个写入方且没有读取方，所以其实不存在并发问题，所以不用加锁。在对同一个内存区域（同一个struct或者切片里面的同一格数据）有并发的读/写操作时，就需要mutex

3. `FetchAll` 要求返回顺序 = 传入顺序。如果不要求呢（谁先完成谁先返回），
   实现会简单还是复杂？调用方会更方便还是更麻烦？
   回答：如果无序返回的话，首先会造成测试用例结果随机成功/失败，比如我测试用例里写的是期望返回的第一个数据是“/a”，那根据执行时间（假设真的走网络了），“/a”有可能在第一个，也有可能在第n个。除了测试外，对实际调用者也会造成使用的不方便：调用者给了2个url，“/a”, "/b"， 不按照输入结果排序的话，他不知道结果里哪个是“/a”的结果，哪个是"/b"的结果。


- 反直觉：
- 踩的坑：
- 没搞懂：

---

## D9 · sync、内存模型、竞态（2026-08-25）

讲解见 `lessons/D9.md`，练习在 `internal/racefix/`（埋了 6 个并发问题）。
先跑 `go run ./cmd/syncdemo`，第 ⑦ 段是题眼（happens-before）。

**小题 A · 并发语义谜题**

五段，**先不要跑**，用脑子推。

```go
// ① 这段会不会一直跑下去？为什么？
var done bool
func a() {
	go func() { time.Sleep(time.Millisecond); done = true }()
	for !done {
	}
	fmt.Println("退出了")
}

// ② 下面哪几行有问题？
type S struct {
	mu sync.Mutex
	n  int
}
func (s S) Inc()      { s.mu.Lock(); s.n++; s.mu.Unlock() }   // (1)
func (s *S) Get() int { s.mu.Lock(); defer s.mu.Unlock(); return s.n }  // (2)
func (s *S) Both()    { s.mu.Lock(); defer s.mu.Unlock(); s.Get() }     // (3)

// ③ 这两段等价吗？
//   A: var n atomic.Int64;  n.Add(1); n.Add(1)
//   B: var mu sync.Mutex; var n int64
//      mu.Lock(); n++; mu.Unlock(); mu.Lock(); n++; mu.Unlock()
// 再问：如果要保证「total 和 sum 始终匹配」，能用两个 atomic 吗？

// ④ 这个双检锁错在哪？（Java 里同样的写法也是错的）
var loaded bool
var cfg *Config
var mu sync.Mutex
func Load() *Config {
	if !loaded {
		mu.Lock()
		if !loaded {
			cfg = &Config{Name: "x"}
			loaded = true
		}
		mu.Unlock()
	}
	return cfg
}

// ⑤ -race 报不报？测试过不过？
func e() {
	m := map[string]int{}
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() { m[fmt.Sprint(i)] = i })
	}
	wg.Wait()
	fmt.Println(len(m))
}
```

我的答案：

```
① 会 一直跑，因为这里没有任何同步和锁的机制，主流程里的for循环看不见go routine里的值改动，go需要锁来保证"happens-before"

② 有问题的是：(   1     )，因为函数接收者是***S***，意味着调用这个函数的时候S的实例会被拷贝一份，而Mutex不能被拷贝

③ A 和 B 不等价，因为B调了两次mu.Lock()，mutex在一个go routine里不能被重入
   两个 atomic 不能 保证 total 和 sum 匹配，因为一个atomic实例只能保证对自己的操作是原子的，两个实例之间的操作没有保证

④ 1092行`if !loaded`的读取是没有锁保证的，可能导致某2个go routine看到它是nil，都去建connection了

⑤ -race 报： 对map并发写          ；不带 -race 时会：panic因为对map并发写  
```

为什么（①④⑤ 请说清楚）：
①已经在上面回答了

②已经在上面回答了

③已经在上面回答了

④已经在上面回答了

⑤已经在上面回答了


实际跑完的验证结果（对了/错在哪）：

**① ⚠️ 结论错了（实测【退出了】，1ms），但理由是对的。**

我答「会一直跑」。实测这个 Go 版本没有把 `done` 提到循环外，循环 1ms 就退出了。

但「没有同步就没有 happens-before」这个理由**完全正确**。所以真正的答案是：

> **这段代码是未定义行为。它今天退出了，不保证明天还退出。**

换个 Go 版本、优化级别、CPU 架构，行为都可能变。`-race` 会照样报警 ——
**它判的是「有没有 happens-before」，不是「这次跑对没跑对」**。

⭐ **「跑对了」不等于「是对的」。** 这是并发代码和普通代码最大的区别：
普通代码测试通过基本就没问题，并发代码测试通过什么也证明不了。

**② ⚠️ 漏了 (3)。**

(1) 对 —— 值接收者拷贝了 `sync.Mutex`，`go vet` 的 `copylocks` 会抓。

**(3) 也有问题 —— 自锁死**（实测卡住了）：

```go
func (s *S) Both() {
	s.mu.Lock()          // ← 第一次
	defer s.mu.Unlock()  // ← 要到函数结束才解
	s.Get()              // ← Get 内部又 s.mu.Lock() → 死
}
```

修法就是那条惯例：**导出方法加锁，内部方法不加锁**（命名带 `Locked` 后缀）。

**③ ❌ 前半答错了，理由也错了；后半 ✅ 对。**

我说「B 调了两次 Lock，mutex 不能重入」—— **那不是重入**。

```go
// B：先解锁再加锁，锁在两次之间是【空闲】的 → 完全合法
mu.Lock(); n++; mu.Unlock();   mu.Lock(); n++; mu.Unlock()

// 真正的重入：还没解锁就再次加锁（嵌套）
mu.Lock()
mu.Lock()      // ← 死
```

**所以 A 和 B 是等价的**：都是两次各自原子的自增，都不保证「两次合起来原子」。
区别只有性能（atomic 更快）。

> 判断方法：**画一条时间线，看 `Unlock` 有没有在第二次 `Lock` 之前**。
> ②(3) 的 `Both` 是真重入（`defer Unlock` 要到函数结束才执行），B 不是。

后半「两个 atomic 之间没有原子性」✅ 完全正确。

**④ ⚠️ 描述的是错误的失败模式。**

我答「可能导致 2 个 goroutine 都去建 connection」—— **不会**。
里面还有一层 `if !loaded` 且在锁保护下，**双重检查的「第二重」正是防这个的，它有效**。

真正的问题是**可见性**：goroutine B 在锁外读到 `loaded == true`，然后读 `cfg`，
这两次读都没有 happens-before 保护，所以可能看到：

- `loaded` 已是 true 但 `cfg` 还是 nil
- 或 `cfg` 非 nil 但它指向的对象**字段还没写完**（`Name` 是空串）

**「初始化执行了两次」不是这个 bug 的症状，「拿到半成品」才是。**
Java 里同样的代码要给 `loaded` 加 `volatile`，`volatile` 建立的正是 happens-before。

**⑤ ⚠️ 两处要精确。**

「`-race` 报对 map 并发写」✅ 对。「不带 -race 会 panic」有两个偏差：

| 我说的 | 实际 |
|---|---|
| **panic** | **`fatal error`** —— `recover()` 拦不住，整个进程死 |
| 会（必然） | **概率性**：实测 20 次里崩了 7 次，13 次静默通过 |

只有 10 条 goroutine、每条写一次，窗口很窄。**「没崩」不代表没问题** —— 呼应 ① 那条。


**小题 B · 设计题（写在下面，不用写代码）**

1. D8 说「数据有明确的主人就用 channel」，D9 说「共享状态用 mutex」。
   `racefix` 里的 6 个问题，你分别选了什么？**有没有哪个你觉得两种都行？**
   如果有，说说取舍。
   回答：已经在代码里写注释了

2. `Registry.Snapshot` 返回内部 map 是个 bug。但「每次都深拷贝」在 map 很大时很贵。
   除了深拷贝，还有什么办法让调用方拿到一致的视图？（提示：想想 D5 的接口和 D8 的 channel）
   回答：可以另外定义一个interface，里面只包含一些必要的，对map的只读方法，比如Len，Get之类，
   Snapshot方法的返回类型改成这个接口。这个接口的实现可以定义一个非导出的struct，struct内部有个map（就是Snapshot现在要返回的那个），然后把map本身的方法包装一下。

3. `go vet` 抓到了 `WaitGroup.Add called from inside new goroutine`，
   但没抓到另外 5 个。**为什么这个能静态分析出来，别的不行？**
   这对「该依赖工具到什么程度」有什么启示？
   回答：因为go vet是静态分析工具，只能分析静态代码，不能分析动态代码。而-race是动态分析工具。没有一个工具能完美得发现所有代码问题，开发时应该多借鉴一些最佳实践，尽量提早发现问题 - 能在编译阶段发现的，就不要等到运行时，能在test/CICD时候发现的，就不要留到生产环境。

**小题 B 批改**

**1 · ⚠️ 没答到点上。** 注释确实写了，但题目问的是「**有没有哪个你觉得两种都行**」。

我的看法：**问题 6（Aggregate 的 total 累加）两种都行。**
- `atomic.Int64`：简单、快（我选的）
- 也可以让每条 goroutine 把结果发到 channel，由一条协调 goroutine 累加

取舍：atomic 更简单；channel 版本在「累加逻辑变复杂」（同时更新 total、count、max）时
更清楚，因为所有更新都在一处、天然一致。**只累加一个数时 atomic 是对的选择。**

**2 · ⚠️ 方向对了一半 —— 而另一半正好是 Claude 那半。**

我提的「只读接口 + 非导出 struct 包装 map」解决了「调用方改不了」，
**但没解决「一致性」**：那个 struct 里的 map 如果是内部那张，写者还在改它 → 照样竞态；
如果是拷贝，就还是付了拷贝代价。

真正零拷贝又一致的办法是 **copy-on-write**：

```go
type Registry struct {
	mu sync.Mutex                       // 只保护写
	m  atomic.Pointer[map[string]int]
}

func (r *Registry) Add(name string) {
	r.mu.Lock(); defer r.mu.Unlock()
	next := maps.Clone(*r.m.Load())     // 写的时候拷贝
	next[name]++
	r.m.Store(&next)                    // 原子换指针
}
```

**写者永远不改已发布的 map，而是造新的换指针** → 老快照从发布起就没人写过，天然安全。

⭐ **但裸 COW 有个洞**（我追问出来的）：`*r.m.Load()` 就是个普通 map，调用方照样能写。
实测：改完之后内部数据被污染成了 `map[svc:9999 注入的键:1]`。

**Go 没有 const / immutable / readonly —— 「不可变」永远靠封装，不靠类型系统。**

所以完整方案是 **COW + 只读视图**（我的那半 + Claude 的那半）：

```go
type View struct{ m map[string]int }   // ← 小写字段，包外碰不到
func (v View) Get(k string) int  { return v.m[k] }
func (v View) Len() int          { return len(v.m) }
func (v View) Range(f func(string, int)) { for k, n := range v.m { f(k, n) } }

func (r *Registry) Snapshot() View { return View{m: *r.m.Load()} }  // 零拷贝
```

三个方案的取舍：

| 方案 | 读成本 | 写成本 | 适合 |
|---|---|---|---|
| `maps.Clone` 每次读都拷（我现在的实现） | **O(n)** | O(1) | 读写都不频繁、map 小 —— **先用这个** |
| COW + 只读视图 | **O(1)** | O(n) | 读远多于写、map 大 |
| 每个方法加锁的活视图 | O(1) | O(1) | 不要「一致快照」，只要「当下的值」 |

补两条：**封装边界是「包」不是「类型」**（同包代码照样能碰 `v.m`）；
**零拷贝共享的前提是数据不可变**，只要还有人可能改就必须拷贝或加锁。

**3 · ✅ 对，但可以更锋利。**

「静态 vs 动态」「没有工具能发现所有问题」「能早发现就别晚」都对。更锋利的说法是：

> **`WaitGroup.Add` 那个是【语法模式】，另外 5 个是【语义问题】。**

`go func(){ wg.Add(1) ... }()` —— 不需要任何运行时信息，**看代码形状就能判定**。

而「这两条 goroutine 会不会同时碰同一块内存」需要知道哪些 goroutine 并发运行、
指针指向哪、map 是不是同一个 —— **这在一般情况下不可判定**（别名分析、可达性分析）。

⭐ **判据：这个错误能不能只看「代码长什么样」就认出来？**
能 → 静态工具可以抓（`copylocks`、`printf`、`SA4005`、`SA4023`）；
不能 → 只能运行时抓（`-race`）或靠人 review。

这和 D6 那句「覆盖率证明不了测试有用」是同一个道理。


- 反直觉：
小题A④，即使“cfg=xxx”比“loaded=true”先赋值，另一个go routine在读到**loaded=true**之后再去读**cfg**也有可能是nil。编译器或 CPU 为了优化性能，可能会将锁内部的赋值指令重排。比如先执行了 loaded = true，还没来得及初始化 cfg 的字段。此时，另一个go routine执行到外层的 if !loaded，发现 loaded 已经是 true 了，于是直接返回了尚未初始化完毕的、残缺的 cfg 指针。当它使用这个 cfg 时，就会发生不可预期的崩溃或错误。

- 踩的坑：
小题A①，代码可能死循环，也有可能能结束。这和go的版本，cpu等运行时环境都有可能有关系。这次能跑过，不一定说明代码本身就毫无问题。
小题A③，误解了“mutex不能被重入”的“重入”的含义。我以为一个go routine里不能对一个mutex进行两次Lock操作。“重入”是指在一对“Lock Unlock”之间，不能再插入一个“Lock”

- 没搞懂：

---

## D10 · context 与并发模式（2026-08-27）

讲解见 `lessons/D10.md`，练习在 `internal/pipeline/`。
先跑 `go run ./cmd/ctxdemo`，第 ⑤ 段是题眼（cancel 停不掉任何东西）。

**小题 A · context 语义谜题**

五段，**先不要跑**，用脑子推。

```go
// ① 子的超时比父长
func a() {
	parent, cancelP := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelP()
	child, cancelC := context.WithTimeout(parent, 10*time.Second)
	defer cancelC()

	start := time.Now()
	<-child.Done()
	fmt.Println(time.Since(start).Round(10*time.Millisecond), child.Err())
}
// 输出是？

// ② 取消的方向
func b() {
	p, cancelP := context.WithCancel(context.Background())
	c, cancelC := context.WithCancel(p)
	defer cancelP()

	cancelC()
	fmt.Println("子:", c.Err(), " 父:", p.Err())
}

// ③ 这段会泄漏吗？为什么？
func c() <-chan int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	out := make(chan int)
	go func() {
		defer close(out)
		for i := range 3 {
			select {
			case out <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
func caller() {
	ch := c()
	fmt.Println(<-ch)   // 只读一个就走
}

// ④ 下面哪个能真的取消掉正在跑的活？
func d(ctx context.Context) {
	// (1)
	go func() { for { heavyCompute() } }()

	// (2)
	go func() {
		for {
			select {
			case <-ctx.Done(): return
			default: heavyCompute()
			}
		}
	}()

	// (3)
	resp, _ := http.Get("https://example.com/slow")
	_ = resp

	// (4)
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://example.com/slow", nil)
	resp2, _ := http.DefaultClient.Do(req)
	_ = resp2
}

// ⑤ 这段代码有什么问题？（不止一个）
type Server struct {
	ctx context.Context
	db  *sql.DB
}
func NewServer(ctx context.Context, db *sql.DB) *Server {
	return &Server{ctx: ctx, db: db}
}
func (s *Server) Handle(w http.ResponseWriter, r *http.Request) {
	uid := s.ctx.Value("userID").(string)
	rows, _ := s.db.QueryContext(s.ctx, "SELECT ...", uid)
	_ = rows
}
```

我的答案：

```
① 输出 = 类似“已超时”之类的。

② 输出 = 子：已经cancel, 父：没有cancel

③ 不会 泄漏，因为超过timeout时间后（1小时），<-ctx.Done()会返回，go routine结束，对应的defer里面会close(out)。

④ 能真正取消的是：( 2  4     )

⑤
问题1：s.ctx.Value("userID")用了普通字符串做value context的key，不能保证唯一，有可能被其他人覆盖，这里应该用非导出的struct实例作为key，比如 type myKey = struct{}, 然后用"myKey{}"作为key。
问题2: 把context作为一个字段存到了Server这个struct里。context通常不应该被放到struct里，而是应该作为一个参数传到需要的方法里（而且通常应该作为方法的第一个参数）。这是因为，context生命周期往往是代表一次操作/一个客户端请求，而struct的生命周期有可能是整个进程的。在上面的示例里，Server代表着服务器和数据库连接的进程，假如context支持timeout，那么在某次请求timeout后，整个进程和数据库的连接都不可用了。
```

为什么（③④⑤ 请说清楚）：
①
因为父context的超时时间是50ms，虽然子context定义的超时时间远大于父的，但实际不能超过父的。

②
父context的cancel会传递到所有子context，反之则不会向上传播

③
已经在上面回答

④
(2)能被取消，是因为当context被cancel时，case <-ctx.Done()这行被执行到，go routine返回，不会执行下一次的heavyCompute()，但是如果当前heavyCompute正在跑，则只能等。
(4)能被取消，是因为http.NewRequestWithContext创建的request本身会响应context的cancel

⑤
已经在上面回答


实际跑完的验证结果（对了/错在哪）：

**① ✅ 对**，实测 `50ms context deadline exceeded`，理由也对。

**② ✅ 完全正确。**

**③ ⚠️ 结论对（不泄漏），但理由错了 —— 而且错得很有价值。**

我的理由：「超过 timeout（1 小时）后 `<-ctx.Done()` 返回，goroutine 结束」。
**不会等 1 小时。** 实测：

```
函数已返回（此时 cancel 已经执行过了）
   [A] ctx 已取消 → goroutine 退出       ← 立刻，不是 1 小时后
```

关键在 **`defer cancel()` 的执行时机：它在 `c()` 这个函数返回时就执行了**，
而不是在返回的 channel 被读完之后。所以链条是：

```
c() 返回 → defer cancel() 立刻触发 → ctx 已经取消
        → 那条 goroutine 第一次 select 就走 <-ctx.Done() 分支 → 退出
```

**这个 ctx 从函数返回那一刻起就是死的**，那 1 小时超时形同虚设。

反过来验证 —— **去掉 `defer cancel()` 才是真泄漏**：

```
50 次调用后 goroutine: 基线 1 → 现在 51
⚠️ 它们都卡在 out <- 1 上，要等【1 小时】超时才退
手动 cancel 之后: 1  ← 立刻退了
```

⭐ **这题真正的教训**：在函数里创建 ctx、又把 channel 返回给调用方，**是个设计错误** ——
ctx 的生命周期（函数作用域）和 channel 的生命周期（调用方决定）对不上。
正确做法是**让调用方传 ctx 进来**，`Source`/`Stage`/`Merge` 的签名就是对的。

**④ ⚠️ (2) 的措辞要精确，(4) 漏了前提。**

**(2)** 「如果当前 heavyCompute 正在跑，则只能等」—— **完全正确**，这正是「协作式」的含义。
但严格说 **(2) 不算「真正取消」，它是「不再开始下一次」**。真要中途停，
`heavyCompute` 内部也得检查 ctx（比如分块处理，每块之间 check 一次）。

**(4)** 「NewRequestWithContext 创建的 request 本身会响应 cancel」—— **方向对，主语不准**。

响应取消的不是 `Request`，是 **`http.Client.Do` 在传输层**：ctx 取消时它会
**中止底层 TCP 连接的读写**。所以：

- ✅ **客户端立刻返回错误**，不再等
- ⚠️ **服务端可能还在处理那个请求** —— 它只是发现连接断了

（这正好是小题 B 第 1 问的答案。）

**⑤ ⚠️ 找到的两个都对，但还有第三个。**

问题 1（string 当 key）和问题 2（ctx 存进 struct）都答对了，
问题 2 的解释很到位 —— 「Server 代表进程，context 代表一次请求，生命周期对不上」。

**漏掉的第三个**：

```go
uid := s.ctx.Value("userID").(string)
                              ^^^^^^^^ 单值类型断言，没用 comma-ok（D5 §5）
```

ctx 里没有这个值时 `Value` 返回 nil，断言直接 **panic** ——
一个 HTTP handler 因为取不到 userID 就崩掉整个请求。

而且这里更该做的是**用 `r.Context()` 而不是 `s.ctx`**：

```go
func (s *Server) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()              // ⭐ 请求级 ctx，客户端断开时自动取消
	uid, ok := UserIDFrom(ctx)      // 类型安全的包装函数
	if !ok {
		http.Error(w, "未认证", http.StatusUnauthorized)
		return
	}
	rows, err := s.db.QueryContext(ctx, "SELECT ...", uid)
	...
}
```

**`r.Context()` 是 Go 里获取请求级 ctx 的标准方式**（D11 会正面讲）。


**小题 B · 设计题（写在下面，不用写代码）**

1. D10 §5 说「取消不会停掉任何东西」。那 `db.QueryContext(ctx, ...)` 被取消时，
   **数据库那边发生了什么**？你的连接会怎样？（提示：想想「不等它了」和「让它停下」的区别）
   回答：连接可能会被中断（取决于db的实现，有没有正确响应context的cancel），数据库那边有没有中断正在执行sql也得看数据库的实现。

2. `Run` 在取消时返回 `nil, ctx.Err()` —— 已经算出来的部分结果被丢弃了。
   什么场景下这是对的？什么场景下应该**返回部分结果 + 错误**？
   如果要返回部分结果，函数签名该怎么变？
   回答：这个看业务场景，如果需要所有的任务都成功执行完才能进行下一步的话，那就应该fail-fast。反之，如果允许部分失败，则应该返回成功的那些+错误的那些（error应该包含错的job本身和错误信息）。

3. 讲义说「不要把 ctx 存进 struct」，但也说「例外确实存在」。
   举一个你觉得**可以**存的场景，说明为什么它不违背「context 是请求域的」这条。
   回答：如果我们用一个struct来做请求的track，比如里面包含请求唯一id，请求开始时间，结束时间，每一个处理它的service name，这时候就可以把ctx放里面

---

**小题 B 批改**

**1 · ⚠️ 说对了「取决于实现」，但把两件【确定】的事说成了不确定。**

实际是三层，前两层确定：

| 层 | 发生什么 | 确定吗 |
|---|---|---|
| **① 我的 Go 代码** | `QueryContext` 立刻返回 `context.DeadlineExceeded` | ✅ 确定 |
| **② 连接** | `database/sql` 把这条连接**标记为坏的并关闭**（不还回池子） | ✅ 确定 |
| **③ 数据库服务端** | 那条 SQL **可能还在跑** | ❌ 取决于驱动 |

实测「不等它了 ≠ 让它停下」：

```
客户端: 30ms 后返回, err = context deadline exceeded
  ⚠️ 此刻服务端【还在跑】
  200ms 后：服务端把那 200ms 的活【完整跑完了】
```

⭐ **第 ② 层最值得记，因为它有性能后果**：取消一次查询 ≈ **销毁一条连接**。
高频超时会让连接池不断重建连接 —— **你以为设超时是在保护自己，实际可能在压垮数据库**。

第 ③ 层：好驱动会另开一条连接发取消指令（pgx 发 `CancelRequest`，MySQL 发 `KILL QUERY`），
但那是**额外请求**，也可能失败。所以：**超时保证「你不再等」，不保证「对方停下来」**。
一个 60 秒的慢查询，你 3 秒超时之后，数据库那边很可能还在烧满 60 秒。

**2 · ✅ 判断对了，但没答「签名该怎么变」—— 那才是设计题的落点。**

三种形态：

```go
// A：现在这样 —— 全有或全无
func Run(...) ([]int, error)

// B：部分结果 + 错误（最直接）
func Run(...) (done []int, err error)     // err != nil 时 done 也可能非空

// C：结果里带上每一项的成败（最清楚）
type Item struct { In, Out int; Err error }
func Run(...) ([]Item, error)             // error 只表示「整体级别的失败」
```

**B 有个隐蔽的坑**：Go 的惯例是「`err != nil` 时其他返回值不可信」，
所以调用方看到 `err != nil` 通常直接 return，辛苦保住的部分结果被无声丢弃。
**用 B 就必须在文档里大写强调。**

**C 更符合 Go 惯例** —— D8 的 `crawl.Result{URL, Links, Err}` 就是这个形态。
**错误是数据的一部分，不是控制流**（D9 §9.8）。

**3 · ✅ 抓到了正确的判据，但举的例子恰好是反例。**

「请求 track struct 是请求域的」—— 判据抓对了。**但恰恰不该存 ctx，因为方向反了**：
request id、时间这些信息**应该放进 ctx**，而不是「ctx 放进那个 struct」。
我描述的其实是 `WithValue` 的场景。

真正合适的例子是**「这个 struct 本身就代表一次操作，且不会被复用」**：

```go
// ✅ 每次调用新建一个，用完就丢
type queryBuilder struct {
	ctx   context.Context      // 这次查询的 ctx
	table string
	where []condition
}

func (db *DB) Query(ctx context.Context) *queryBuilder {
	return &queryBuilder{ctx: ctx}          // ⭐ 每次都是新的
}
q := db.Query(ctx).Where("a=1").Limit(10)   // 链式调用，ctx 得跟着走
```

它不违背规则，是因为**这个 struct 的生命周期 = 一次请求，和 ctx 完全对齐**。
而 `Server`、`Repository`、`Client` 是**进程级**的，生命周期跨越无数请求 —— 存 ctx 才是错的。

⭐ **判据：这个 struct 每次请求都新建一个吗？**
是 → 可以存（但优先还是传参数）；否 → 绝对不能存。

标准库的例子：`http.Request` 内部就有 ctx（`r.Context()` 取的就是它）——
**因为每个请求都是新的 Request**。


- 反直觉：
- 踩的坑：
pipeline.go里的merge方法，我一开始merge操作其实是串行的，写法如下：
```go
	go func() {
		defer close(out)
		
		for _, in := range ins {
			for v := range in {
				select {
				case <-ctx.Done():
					return
				case out <- v:
				}
			}
		}
	}()
```
- 没搞懂：

---

## D11 · net/http 服务端（2026-08-30）

讲解见 `lessons/D11.md`，练习在 `internal/httpx/`。
先跑 `go run ./cmd/httpdemo`，第 ⑥ 段是题眼（四个超时默认是「永不超时」）。

**小题 A · HTTP 语义谜题**

五段，**先不要跑**，用脑子推。

```go
// ① 这个 handler 返回什么状态码？响应体是什么？
func a(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"partial":`)
	if err := doSomething(); err != nil {          // 假设这里出错了
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "出错了")
		return
	}
	fmt.Fprint(w, `true}`)
}

// ② 中间件顺序：下面两种写法，日志里记到的状态码分别是什么？
//   A: Chain(mux, Logging, Recover)
//   B: Chain(mux, Recover, Logging)
// （mux 里的 handler 会 panic）

// ③ 这段代码有什么问题？
func c() {
	resp, err := http.Get("https://api.example.com/data")
	if err != nil {
		return
	}
	var result Data
	json.NewDecoder(resp.Body).Decode(&result)
}

// ④ 下面哪个 ResponseWriter 包装能正常支持 SSE（需要 Flush）？
type w1 struct{ http.ResponseWriter; status int }
func (w *w1) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }

type w2 struct{ rw http.ResponseWriter; status int }
func (w *w2) Header() http.Header { return w.rw.Header() }
func (w *w2) Write(b []byte) (int, error) { return w.rw.Write(b) }
func (w *w2) WriteHeader(c int) { w.status = c; w.rw.WriteHeader(c) }

// ⑤ 这个服务上线后会出什么问题？（不止一个）
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		process(body)
		w.Write([]byte("ok"))
	})
	http.ListenAndServe(":8080", mux)
}
```

我的答案：

```
① 状态码 =  200      响应体 = {"partial":出错了true}，content type是application/json

② A 记到的状态码 =   500     B 记到的状态码 =  记不到

③
问题1: 没有超时设置
问题2: err的时候，没有读完response body，会造成http/1.1的“连接复用”不可用

④ 能支持 SSE 的是：(    w1    )

⑤
问题1：没有设置超时
问题2: io.ReadAll碰到大请求时（比如传一个100G的文件），会直接打爆服务器内存。process(body)也一样的问题。
```

为什么（①②④⑤ 请说清楚）：
①
fmt.Fprint(w, `{"partial":`)这行已经让response code是200了
并且也把content-type=application/json这个响应头写出去了。后面会接着打印出“出错了”和“true}”

②
A的写法，请求/响应流过的顺序是
请求：logging -> recover -> mux
响应 mux -> recover -> logging
当mux发生panic时，不管是在请求阶段还是在响应阶段，都会被recover里的defer里面的"if recovered := recover(); recovered != nil"捕获到，假设它里面会把response code写成500，然后再回到logging，logging就能抓到500的code

B的写法，求/响应流过的顺序是
请求：recover -> logging -> mux
响应 mux -> logging -> recover
假设logging的defer里面没有“recover()”的处理，那么它本身也被panic打穿了，直到回到recover层。

③
已经在上面回答

④
w1用了“嵌入”，当要支持flush还需要额外包装下flush方法。
w2只是一个包含了一个http.ResponseWriter类型字段的struct，编译器不知道它会有flush方法

⑤
已经在上面回答


实际跑完的验证结果（对了/错在哪）：

**① ⚠️ 状态码和 Content-Type 对，响应体多了一截。**

真实 server 的结果：

```
状态码       = 200                      ✅ 我答对了
Content-Type = "application/json"        ✅ 我答对了（第二次 Set 无效）
响应体       = "{\"partial\":出错了"      ⚠️ 我答的是 {"partial":出错了true}
```

**没有 `true}`** —— `if` 分支里 `return` 了，`fmt.Fprint(w, `true}`)` 那行根本执行不到。

服务端日志里还有一条：

```
http: superfluous response.WriteHeader call from main.a
```

标准库检测到了「重复 WriteHeader」并打日志，但**不 panic、不改状态码** —— §3 说的静默失效
（准确说是「日志里有，客户端无感」）。

> ⭐ **顺带一个重要发现**：`httptest.ResponseRecorder` 在这里**和真实 server 不一致**。
> 它的 `Header()` 返回的是**当前的 map**，第二次 `Set` 会覆盖（所以 recorder 里显示 text/plain）；
> 真实 Server 在第一次 `Write` 时就把 header 序列化发出去了，之后改 map 完全无效。
>
> **测 header 覆盖这类问题时，`httptest.NewRecorder()` 会骗你，要用 `httptest.NewServer`。**

**② ✅ 全对，而且解释准确。**

```
A（Logging 外, Recover 内）: [Logging 记到 500]
B（Recover 外, Logging 内）: []  ← 空的
```

「Logging 本身也被 panic 打穿了」—— **精确**。`next.ServeHTTP` panic 之后，
Logging 后面的 `log(...)` 根本不执行，所以**一条日志都没有**，不是「记到了错的值」。

这正是讲义 §4 的取舍：Logging 在外能记到 500，但它自己 panic 就没人兜。

**③ ✅ 两个都对**（没超时、body 没读完影响连接复用）。补两个漏掉的：

```go
resp, err := http.Get(...)                    // ③ 忽略了 resp.Body.Close() —— 连接泄漏
json.NewDecoder(resp.Body).Decode(&result)    // ④ 没检查 Decode 的错误
```

`bodyclose` linter 会抓第三条。

**④ ❌ 两个都不支持。**

```
真实 ResponseWriter: true
w1（嵌入接口）:      false
w2（手动实现三个）:  false
```

**没有 `Flush` 方法的类型，都不满足 `http.Flusher`。** 我的解释是对的
（「w1 还需要额外包装下 flush 方法」），但选项选错了 —— 题目问「哪个能**正常支持**」，
答案是「都不能，两个都要补 `Flush`」。

⭐ 这题的用意：**「嵌入接口」和「手动实现三个方法」在丢失可选接口这件事上是等价的。**
很多人以为嵌入更「自动」一些，其实不是。

**⑤ ⚠️ 两个都对，还漏了三个。**

```go
mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)        // ③ 忽略了错误
	process(body)                         // ④ 没用 r.Context()
	w.Write([]byte("ok"))
})
http.ListenAndServe(":8080", mux)         // ⑤ 没有 Recover 中间件
```

- **③ 忽略 `ReadAll` 的错误** —— 客户端中途断开时 `body` 是残缺的，却被当完整数据 process 了
- **④ 没用 `r.Context()`** —— `process(body)` 很慢时，客户端早断了它还在跑（D10 §5）
- **⑤ 路由没限制方法** —— `"/upload"` 是旧式模式，`GET`/`DELETE` 也能触发上传逻辑


**小题 B · 设计题（写在下面，不用写代码）**

1. `Logging` 需要包装 `ResponseWriter` 才能拿到状态码。这个包装会丢掉
   `http.Flusher` / `http.Hijacker` 等可选接口（D5 §5 的可选接口模式）。
   **你打算怎么处理？** 列出你知道的办法和各自的代价。
   回答：
   1. 直接在 `Logging` 中实现 `http.Flusher` / `http.Hijacker` 等可选接口。
      代价：需要在 `Logging` 中实现 `Flush` 方法，可能会增加代码量。
   2. 实现Unwrap方法，里面返回底层的ResponseWriter。

2. `RateLimit` 现在是全局一个桶。要改成**按 IP 限流**，需要考虑哪些问题？
   （提示：想想 map 会不会无限增长、锁的粒度、以及「IP 从哪来」）
   回答：可以用一个map，key是ip，value是桶里的令牌数。这里要考虑ip太多map太大问题。可以跑一个go routine，每过n毫秒后，看看有没有超过一定时间没有被访问过的ip，有就从map里删掉它。对map操作的时候还要考虑并发锁的问题，理论上不同的ip之间不需要互相锁。我们可以健这么一个struct:
   ```go
   type ipBucket struct {
	mu sync.Mutex
	tokens int
   }
   ```
   map的value存的是这个struct实例的指针，这样修改token数目的时候，只需要对每个ip对应的这个实例加锁。

3. 讲义 §7 说四个超时必须配。但 `WriteTimeout` 有个陷阱：它从**读完请求头**开始算，
   而不是从 handler 开始写响应算。**什么样的接口会撞上这个坑？** 怎么办？
   回答：处理请求非常慢，但是最终返回的数据很少（写response快）的接口会碰到这个问题。碰到这个问题后，可以：
   a. 增大timeout时间
   b. 优化请求处理本身，比如，
       用多个go routine能不能加速？
	   能不能先返回一个task id，再提供一个api根据task id来查询进度/结果


---

**小题 B 批改**

**1 · ⚠️ 两个办法都对，但漏了「代价」，而且第一条和讨论后的结论矛盾。**

我写「直接实现 `http.Flusher` / **`http.Hijacker`**」—— 但 **`Hijacker` 不该裸转发**：
转发之后连接被接管，`status`/`bytes` 再也不会更新，日志会变成 `status=0 bytes=0`
而客户端实际收到了 418。**转发了就是在说谎。**

完整的四个选项和代价：

| 办法 | 代价 |
|---|---|
| ① 转发 `Flush` | 每个可选接口写一遍；**只对「不夺走控制权」的接口安全** |
| ② 提供 `Unwrap` | 只对用 `ResponseController` 的调用方有效；**直接类型断言的老代码享受不到** |
| ③ ①+②（我代码里实际做的） | 兼容新老调用方，两处都要维护 |
| ④ 用 `httpsnoop` | 代码生成穷举所有组合，一个不丢；多一个依赖 + 不透明 |

⭐ 还有个更本质的取舍：**「诚实地不支持」也是合理选择** —— 不转发 `Hijacker`，
让 handler 的断言拿到 false 走降级分支，比转发之后给出错误的统计要好。

**2 · ✅ 答得很好**，`map[string]*ipBucket` + 每 IP 一把锁的思路完全正确。

但漏了提示里的第三点 —— **「IP 从哪来」**，而这是三问里**唯一有安全后果**的：

```go
ip := r.RemoteAddr        // ← TCP 连接的对端地址
```

**前面有 nginx / LB / CDN 时，所有请求的 RemoteAddr 都是那个代理的 IP**
→ 「按 IP 限流」退化成全局限流，一个用户就能打满所有人的额度。

那用 `X-Forwarded-For`？**它可以被客户端随便伪造**：

```
X-Forwarded-For: 1.2.3.4      ← 攻击者每次换一个，无限绕过限流
```

正确做法是配置**「信任几层代理」**，从 XFF 右边往左数：

```
X-Forwarded-For: 伪造的, 真实客户端IP, 代理1, 代理2
                          ↑ 信任 2 层的话，从右数第 3 个才是真的
```

**关键是这个「信任层数」必须是配置项，不能猜。**

另外两个漏掉的细节：

**① 还需要一把外层锁保护 map 本身**

```go
type ipLimiter struct {
	mu      sync.RWMutex           // ⭐ 保护 map 结构（查找/增删）
	buckets map[string]*ipBucket   // 每个 bucket 有自己的锁
}
```

「不同 IP 之间不需要互相锁」是对的，但**读写 map 这个数据结构本身**仍然要锁。
这是**两层锁**：外层 RWMutex（读多写少）+ 内层每桶一把。

**② 清理 goroutine 谁来停** —— 长期运行的 goroutine 要有退出路径（D10）：

```go
select {
case <-ticker.C:   cleanup()
case <-ctx.Done(): return      // ⭐ 否则测试里反复创建 limiter 就泄漏
}
```

**3 · ⚠️ 说对了一类，漏了最典型的一类。**

「处理很慢但响应很小」—— **对**，实测 `/slow` 确实被砍（EOF）。

**但最典型的是 SSE / 长轮询 / 流式响应**（实测，全局 WriteTimeout=200ms）：

```
/slow        ❌ EOF                       ← 我说的那类
/sse         ⚠️ 只收到 45/75 字节，中途断   ← 最典型的一类，我漏了
/sse-fixed   ✅ 完整收到 75 字节            ← SetWriteDeadline 救回来了
```

**它们本来就要长时间保持连接** —— SSE 可能推一整天。`WriteTimeout` 从读完请求头
开始算，所以**不管推得多快，到点就砍**。同类的还有大文件下载、WebSocket 握手前。

我答的两条（增大 timeout、改异步任务）都是**绕**而不是**解**：

- 增大 timeout → Slowloris 防护就废了，所有接口一起变松
- 改异步任务 → 架构改动大，而且不适用于 SSE（它天生就是长连接）

**直接的解法是 `http.ResponseController.SetWriteDeadline`**：

```go
rc := http.NewResponseController(w)
for {
	rc.SetWriteDeadline(time.Now().Add(2 * time.Second))   // ⭐ 每次推送前往后推
	fmt.Fprintf(w, "data: %s

", event)
	rc.Flush()
}
```

⭐ 这是 `ResponseController` 的第二个用途（第一个是穿透包装拿 Flusher）：
**让单个 handler 覆盖全局超时**。

> **全局设紧，个别放宽** —— 比「全局设松」安全得多。


**📄 配套示例：`internal/httpx/events.go`**

按第 3 问写了一个可运行的 SSE 端点（Store 变更实时推送），它同时踩中四个坑：

| 坑 | 在示例里怎么处理 |
|---|---|
| **D11 §7** WriteTimeout 从读完请求头算 | `writeAndFlush` 每次写之前 `SetWriteDeadline` |
| **D11 §5** 包装 ResponseWriter 丢 Flusher | 用 `ResponseController` 而不是 `w.(http.Flusher)` |
| **D10 §5** 客户端断开 | `select` 里有 `<-r.Context().Done()` 分支 |
| **D9 §11** AB-BA 死锁 | 广播在**解锁之后**做，且 `subs` 有自己的锁 |

**两条硬规则**（违反任何一条都会拖垮服务）：

1. **绝不能持有 `Store.mu` 时广播** —— 广播要拿 `subs.mu`，而订阅者可能正好在调
   `Store.Get`（要拿 `Store.mu`），两把锁顺序相反就是 AB-BA 死锁。
   所以 `Put`/`Delete` 里**没有用 `defer Unlock`**，而是显式解锁后再广播。

2. **发送必须非阻塞**（`select` + `default`）—— 一个卡住的客户端（手机切后台）会让
   它的 channel 填满，阻塞发送的话**整个 Store 的写入全卡死**，一个慢客户端拖垮所有人。

「丢事件」是**有意的取舍**：宁可让慢客户端丢几条，也不能让它拖住写入方。
真要保证不丢，就得给每个订阅者一个持久队列（那是另一个系统了）。

**实测验证**（全局 `WriteTimeout=150ms`，事件间隔 200ms —— 每次都超过超时）：

```
响应头: Content-Type="text/event-stream" Transfer-Encoding=[chunked]  ← flush 生效
收到的事件流:
   event: put     data: {"key":"k","value":"a"}
   event: put     data: {"key":"k","value":"b"}
   event: put     data: {"key":"k","value":"c"}
   event: put     data: {"key":"k","value":"d"}
   event: delete  data: {"key":"k","value":""}
✅ 4 次 put + 1 次 delete 一条没丢
✅ 1000 次写入没被慢订阅者阻塞
✅ 10 次连接并断开：订阅者数量 0 → 0（没泄漏）
```

其他值得记的细节：

- **`Subscribe` 返回「取消函数」而不是「取消方法」** —— `context.WithCancel` 的同款设计，
  调用方拿到什么就能取消什么，不用额外记 id
- **取消函数用 `sync.Once` 保证幂等** —— 调用方很可能 defer 一次、出错路径再调一次，
  重复 `close` 会 panic（D8 §3）
- **心跳（`: ping`）** —— 反向代理会掐掉空闲太久的连接；
  `writeWindow` 必须**大于** `heartbeat`，否则心跳还没发连接就被砍了
- **`SetWriteDeadline` 可能返回 `ErrNotSupported`** —— `httptest.ResponseRecorder`
  就不支持。这不是致命错误，继续写就行，只是失去了「单独放宽超时」的能力


- 反直觉：
- 踩的坑：
输出response的时候，如果手动拼json会碰到“注入”问题。我之前是这么写的：
```go
w.Write([]byte({"key":"+ key +","value":"+ value +"})) 
```
在 value 含 " / \n / \ 时会拼出非法 JSON。
应该用
```go
json.NewEncoder(responseWriter).Encode(value)
```
- 没搞懂：
