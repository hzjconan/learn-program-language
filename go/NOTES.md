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
