# Go 问答手册

> 学习过程中我问、Claude 答的 Go 语法/用法问题,按主题归档。
> 和 `lessons/` 的区别：讲义是**成体系的课**,这里是**当场卡住的具体问题**,查阅优先。
>
> 记录规则：不是每次问答都立刻写进来,攒几条一起记,记之前先跟我确认。

## 索引

| # | 问题 | 主题 |
|---|---|---|
| [1](#1-cmd-目录是-go-的命名规范吗) | `cmd/` 目录是 Go 的命名规范吗 | 工程 |
| [2](#2-怎么定义并初始化一个字符串数组) | 怎么定义并初始化一个字符串数组 | 类型 |
| [3](#3-为什么单位表不能写成-const) | 为什么单位表不能写成 `const` | 类型 |
| [4](#4-包级声明的先后顺序有要求吗) | 包级声明的先后顺序有要求吗 | 语法 |
| [5](#5-fmt-的格式化动词速查) | `fmt` 的格式化动词速查 | 标准库 |
| [6](#6-传指针给函数传的是指针本身还是拷贝) | 传指针给函数，传的是指针本身还是拷贝 | 指针 |
| [7](#7-slice--map-传参到底共享了什么) | slice / map 传参到底共享了什么 | 指针 |
| [8](#8-errorsis-比较的是内存地址吗) | `errors.Is` 比较的是内存地址吗 | 错误处理 |
| [9](#9--和--的区别为什么-_--会报错) | `:=` 和 `=` 的区别，为什么 `_ :=` 会报错 | 语法 |
| [10](#10-栈堆和逃逸分析是什么) | 栈、堆和「逃逸分析」是什么 | 运行时 |
| [11](#11-go-怎么表示集合set为什么是-mapkstruct) | Go 怎么表示集合（set）？为什么是 `map[K]struct{}` | 类型 |
| [12](#12-预分配的-cap-会不会白占内存slicesclip) | 预分配的 cap 会不会白占内存？`slices.Clip` | 切片 |
| [13](#13-一个-nil-指针装进-error-接口后为什么就不是-nil-了) | 一个 nil 指针，装进 `error` 接口后为什么就不是 nil 了 | 接口 |
| [14](#14-跑-benchmark-时为什么要加--run-参数) | 跑 benchmark 时为什么要加 `-run` 参数 | 测试 |
| [15](#15-无缓冲-channel-什么时候会死锁) | 无缓冲 channel 什么时候会死锁 | 并发 |
| [16](#16-setwritedeadline两次写之间隔很久会不会超时) | `SetWriteDeadline`：两次写之间隔很久，会不会超时 | 网络 |
| [17](#17-writetimeout-和-setwritedeadline-同时存在时谁说了算) | `WriteTimeout` 和 `SetWriteDeadline` 同时存在时谁说了算 | 网络 |
| [18](#18-包装-sloghandler-为什么会丢字段) | 包装 `slog.Handler` 为什么会丢字段 | 标准库 |
| [19](#19-非导出类型上的大写方法算导出吗) | 非导出类型上的大写方法，算导出吗 | 工程 |

---

## 1. `cmd/` 目录是 Go 的命名规范吗

**一句话**：是社区惯例,不是语言规范。Go 工具链根本不认识 `cmd/`、`pkg/`、`api/` 这些名字。

语言真正强制的只有两条：

- `package main` + `func main()` 的包编译成可执行文件,其他包编译成库。
- 路径含 `internal/` 的包有导入限制(**这条是编译器强制的**)。

`cmd/` 解决的真实约束：**一个目录 = 一个包**,而每个可执行文件都得是 `package main`,所以多个二进制必须各占一个目录,目录名即默认二进制名。收进 `cmd/` 下只是为了让根目录不被淹没。标准库自己就这么干(`cmd/go`、`cmd/compile`)。

**什么时候别用**：

- 单一二进制的小项目 → 根目录直接放 `main.go`。
- 纯库项目 → 根本不需要 `cmd/`。
- `pkg/` 争议更大。那个流传很广的 `golang-standards/project-layout` **不是官方的**(名字有误导性),Go 团队成员公开批评过它把企业级 Java 的目录仪式搬进了 Go。

**原则：目录结构反映「这个项目有哪些包」,从平铺开始,真需要了再加层。** 和 Java 那种一上来就 controller/service/dao 四件套的思路相反。

> 展开版见 [`lessons/D1.md`](lessons/D1.md) 附录 Q2。

---

## 2. 怎么定义并初始化一个字符串数组

```go
// 数组（array）—— 长度固定，且长度是类型的一部分
a := [3]string{"a", "b", "c"}
a := [...]string{"a", "b", "c"}   // ... 让编译器数个数

// 切片（slice）—— 变长，⭐ 99% 的场景用这个
s := []string{"a", "b", "c"}

// 先声明后填
s := make([]string, 0, 3)         // 长度 0，容量 3
s = append(s, "a", "b", "c")

var s []string                    // nil slice，也能直接 append
```

**区别就看方括号里有没有数字**：`[3]string` 是数组,`[]string` 是切片。

### 为什么几乎总是用 slice

**Go 的数组是值类型**——这是从 Java/TS 过来最需要调的认知：

```go
a := [3]string{"a", "b", "c"}
b := a; b[0] = "X"        // 整个数组被复制，a[0] 仍是 "a"

s := []string{"a", "b", "c"}
t := s; t[0] = "X"        // 只复制 slice header，底层数组共享 → s[0] 变成 "X"
```

Java 的 `String[]` 和 TS 的 `string[]` 都是引用类型,**它俩对应的是 Go 的 slice,不是 Go 的 array**。按 Java 直觉写 `[3]string` 传进函数,会得到一次静默的全量拷贝。

再加一条：**长度是类型的一部分**,`[3]string` 和 `[4]string` 是不同类型,不能互相赋值。这让数组在 API 里基本没法用。

数组真正有用的场景很窄：固定尺寸的密码学摘要(`[32]byte`)、需要栈上分配避免 GC 的热点路径、**数组能当 map 的 key 而 slice 不能**。其余一律 slice。

### 常用操作

```go
len(s)                    // 长度
s = append(s, "d")        // ⭐ 必须接住返回值！Go 没有原地 push
s[1:3]                    // 左闭右开
slices.Contains(s, "b")   // 标准库 slices 包（1.21+）
slices.Sort(s)
strings.Join(s, ",")
for i, v := range s { }   // 不要 index 就写 for _, v := range s
```

`s = append(s, x)` 的赋值不能省——`append` 可能返回新的底层数组,忘了接住是新手一号 bug。

---

## 3. 为什么单位表不能写成 `const`

**Go 的常量只能是基本类型：数值、字符串、布尔。slice、map、struct 都不行。**

因为常量必须在**编译期完全求值**,而 slice 需要在运行时分配底层数组。这条限制没有 workaround,包级 `var` 就是标准答案：

```go
const units = []Unit{...}   // ❌ 编译错误
var units = []Unit{...}     // ✅
```

代价是它可变——包内任何代码都能改它。Go 没有 `final`/`readonly`,靠的是不导出 + 约定,不是类型系统。

---

## 4. 包级声明的先后顺序有要求吗

**没有。** 包级的 `var` / `func` / `type` / `const` 可以任意顺序,编译器自己解析依赖。函数里用到的包级变量,声明在函数下面也完全合法。

这点和 C(必须先声明)、JS(`let`/`const` 有 TDZ)都不同,是个小惊喜。

但**惯例**是：数据表放在使用它的函数**上面**,让读者先看到「有这么张表」再看到「怎么用」。顺序不影响编译,影响可读性。

---

## 5. `fmt` 的格式化动词速查

**别背。** 记住下面这 20%，剩下的用 `go doc fmt` 查。

### 先分清函数族

```go
fmt.Printf("x=%d\n", 1)                  // 打到 stdout，要自己写 \n
fmt.Println("x =", 1)                    // 自动加空格和 \n，不用动词
s := fmt.Sprintf("x=%d", 1)              // ⭐ 返回 string
fmt.Fprintf(w, "x=%d", 1)                // 写到任意 io.Writer（文件、HTTP 响应…）
err := fmt.Errorf("读取失败: %w", err)    // 造 error，%w 是包装
```

命名规律：**`S` 前缀 = 返回 String，`F` 前缀 = 写入 Writer，无前缀 = stdout**。记住这条，函数名都不用背。

### 核心动词（覆盖 95% 场景）

| 动词 | 用途 | 输出示例 |
|---|---|---|
| `%v` | **万能默认格式**，任何类型都能吃 | `[1 2 3]`、`{Alice 30}` |
| `%+v` | 同上但 struct **带字段名** ⭐ | `{Name:Alice Age:30}` |
| `%#v` | Go 语法形式，能直接粘回代码 | `main.User{Name:"Alice", Age:30}` |
| `%d` | 整数 | `42` |
| `%s` | 字符串 | `hello` |
| `%q` | **带引号的字符串** ⭐ | `"hello"` |
| `%f` | 浮点，默认 6 位小数 | `3.140000` |
| `%.1f` | 浮点，指定小数位 | `3.1` |
| `%t` | 布尔 | `true` |
| `%T` | **打印类型本身** ⭐ 调试神器 | `units.ByteSize` |
| `%%` | 一个字面的百分号 | `%` |
| `%p` | 指针地址 | `0x14000112028` |

打星的三个是 Go 特有、Java `String.format` 里没有的，最该形成肌肉记忆：

- **`%v` / `%+v`**：不知道用什么就用 `%v`，切片、map、struct、指针都有合理输出。调试打 struct 一律 `%+v`，不带 `+` 只能看到一堆没标签的值。
- **`%q`**：测试里的标配。`got = hello` 和 `got = "hello"` 的区别在于——**后者能让你看见首尾空格和空字符串**。
- **`%T`**：搞不清手上是什么类型时打一发，比猜快。

### 宽度和精度

格式是 `%[flags][宽度][.精度][动词]`：

```go
fmt.Sprintf("%6.2f", 3.14159)   // "  3.14"      宽 6，小数 2 位，右对齐
fmt.Sprintf("%-10s|", "ab")     // "ab        |"  减号 = 左对齐
fmt.Sprintf("%05d", 42)         // "00042"        补零
```

宽度是**最小**宽度，不够补齐，够了也不截断。

### 两条能救命的

1. **`go vet` 会检查 printf 参数匹配** —— 动词和参数类型对不上、数量不对，它都报错。写错不会静默出问题，`make check` 就拦下了。这是相对 Java 的实打实优势：`String.format` 的错误要到运行时才炸。

2. **完整清单一条命令**：`go doc fmt` —— 包文档开头就是完整动词表，比搜索快，且是本地这个版本的准确文档。

---

## 6. 传指针给函数，传的是指针本身还是拷贝

**拷贝。Go 永远传值，没有例外。**

传 `x := &user` 时，函数收到的是**一个新的 8 字节变量，里面装着同一个地址**。

```go
func modifyPointee(u *User)   { u.Name = "改了" }         // 改【指向的对象】→ 调用方可见 ✅
func reassignPointer(u *User) { u = &User{Name: "换了"} } // 改【指针本身】→ 调用方看不见 ❌
```

> **拷贝的是指针，不是指针指向的东西。**
> 能透过它改远端数据，但改不了调用方手里那根「指针」本身。

**和 Java 完全一致**——Java 也是永远传值，传对象时传的是「引用的拷贝」。Go 只是把 `*` 明写出来了。

### 推论：为什么标准库老要你传 `&`

```go
errors.As(err, &pe)          // pe 已是 *ParseError，这里传 **ParseError
json.Unmarshal(data, &cfg)
fmt.Sscanf(s, "%d", &n)
```

因为这些函数**要写回给你**。既然只有值传递，想改调用方的变量就必须拿到那个变量的地址。

**看到标准库要求 `&`，一律是「它要写回给你」。**

> 可运行验证：`go run ./cmd/ptrdemo`（源码 `cmd/ptrdemo/main.go`）。
> 盯地址那一列：函数内部赋值后地址确实变了，返回后调用方的没变 —— 证明改的不是同一个变量。

---

## 7. slice / map 传参到底共享了什么

常听到的「slice、map、chan 是引用类型」**是个偷懒的说法**。精确的说法是：

> Go 永远传值。它们只是**值本身里面就含了一个指针**。

**map / chan**：变量本身**就是**一个指针（`*hmap` / `*hchan`）。拷贝它 = 拷贝一个地址，两边指向同一份数据，所以函数里的写入调用方全都看得见。

**slice**：变量是个**三字段 struct**，传参时整个被拷贝：

```
[ 指向底层数组的指针 | len | cap ]     ← 24 字节
```

指针字段指向同一个底层数组 → **改元素可见**；但 `len`/`cap` 是独立副本 → **改长度不可见**：

```
函数里 s[0]=999:   [999 2 3]      ← 可见
函数里 append:     [999 2 3]      ← 不可见！
```

`append` 一定会改 `len`，而 `len` 只存在于函数里那份拷贝中。

> **这就是为什么 `s = append(s, x)` 的赋值不能省** —— append 返回新的 slice header，必须自己接住。

### 什么时候需要 `*[]T`

只有一种：函数要改变调用方的 slice header 本身（append / 重新切片 / 置 nil）。但**惯用做法是返回新 slice，不传 `*[]T`**：

```go
func appendAll(s []int, vals ...int) []int { return append(s, vals...) }
s = appendAll(s, 4, 5)
```

标准库全是这个形状（`append`、`strconv.AppendInt`、`slices.Delete`）。看到 `*[]T` 出现在 API 里，八成设计味道不对。`*map[k]v` 更是纯噪音——那是在指针上再套指针。

### 一张表

| 类型 | 传参时拷贝了什么 | 改元素可见？ | 改长度可见？ |
|---|---|---|---|
| `int` / `struct` / `[3]int` | 整个值 | — / ❌ | — |
| `*T` | 8 字节地址 | ✅（改 `*p`） | — |
| `map` / `chan` | 8 字节地址 | ✅ | ✅ |
| `[]T` | 24 字节 header | ✅ | ❌ |

**唯一需要单独记的就是最后一行那个 ❌。** 其余都能从「永远传值」推出来。

---

## 8. `errors.Is` 比较的是内存地址吗

**不一定。核心那一步是 `err == target`——【接口相等】，而接口相等 = 动态类型相同 且 动态值相等。**

「动态值怎么比」取决于类型：

| 错误的动态类型 | `==` 实际在比 |
|---|---|
| `*errorString`、`*MyError`（**指针**） | **地址** |
| `MyError`（**struct 值**） | **逐字段比较** |
| `syscall.Errno`（底层 `uintptr`） | **数值** |

「比地址」只是最常见的那种，因为 `errors.New` 和多数自定义错误类型都用指针。

```go
type CodeError struct{ Code int }
func (e CodeError) Error() string { return fmt.Sprintf("code %d", e.Code) }

a, b := CodeError{404}, CodeError{404}
&a == &b            // false —— 两个不同的变量
errors.Is(a, b)     // true  —— 但字段相同，接口相等成立
```

### 完整算法

```go
for {
    // ① 如果 target 可比较，试直接相等
    if isComparable && err == target { return true }

    // ② 如果 err 自己实现了 Is 方法，问它
    if x, ok := err.(interface{ Is(error) bool }); ok && x.Is(target) { return true }

    // ③ 顺着 Unwrap 往下一层，回到 ①
    err = errors.Unwrap(err)
    if err == nil { return false }
}
```

- **`isComparable` 守卫是必要的**：如果 `target` 的类型不可比较（比如 struct 含 slice 字段），`==` 会直接 panic，所以先用反射检查。
- **第 ② 步是逃生舱**：任何错误类型都能实现 `Is(error) bool` 自定义「什么算匹配」，完全绕开值比较。
- 每一层都同时试 ① 和 ②，所以包了五层照样匹配。

### 精髓在第 ② 步：`fs.ErrNotExist` 的跨平台映射

```go
_, err := os.Open("/不存在")
// 错误链：*fs.PathError →(Unwrap)→ syscall.Errno(2)

pe.Err == fs.ErrNotExist          // false —— 类型和地址都不同
errors.Is(err, fs.ErrNotExist)    // true
```

链底是个**操作系统错误码**，和 `fs.ErrNotExist` 这个包级变量毫无关系。能匹配上是因为 `syscall.Errno` 实现了 `Is`：

```go
func (e Errno) Is(target error) bool {
	switch target {
	case fs.ErrNotExist:   return e == ENOENT
	case fs.ErrPermission: return e == EACCES || e == EPERM
	case fs.ErrExist:      return e == EEXIST || e == ENOTEMPTY
	}
	return false
}
```

**这是 Go 做跨平台错误抽象的机制**：各平台的 errno 数值体系完全不同，各自实现 `Is` 映射到同一组 `fs.Err*` 哨兵，于是 `errors.Is(err, fs.ErrNotExist)` 在哪都对。

### 设计启示

值类型的错误会被「字段相同即相等」——对 `CodeError` 这种**分类标签**语义正合适；但对携带位置信息的 `*ParseError` 就不对了，第 3 行和第 3 行的两个不同失败不该被当成同一个。

> **指针接收者给你【身份】语义，值接收者给你【相等】语义。**
> 错误类型几乎总是用指针接收者，除非它就是个纯粹的分类标签。

---

## 9. `:=` 和 `=` 的区别，为什么 `_ :=` 会报错

| | `:=` | `=` |
|---|---|---|
| 作用 | **声明** + 初始化 | 纯**赋值** |
| 变量必须已存在？ | 不能全都已存在 | 必须已存在 |
| 能写类型吗 | 不能（靠推断） | 不适用 |
| 函数外能用吗 | **不能** | 不能（包级只能 `var`） |

### 为什么 `_ :=` 报错

```go
_ := f.Close()   // ❌ no new variables on left side of :=
_ = f.Close()    // ✅
```

`:=` 的硬规则是「**左边至少要有一个新变量**」。而 `_` **不声明任何变量**——它是空白标识符，是「把值扔掉」的语法，不是变量名。所以 `_ :=` 左边一个新变量都没有。

对照着看就清楚了：

```go
a, err := f()    // ✅ a、err 都是新的
b, err := g()    // ✅ b 是新的，err 是【赋值】—— 至少有一个新的，合法
_,  err := h()   // ✅ err 是新的就合法
_, _ := h()      // ❌ 一个新变量都没有
_ = x            // ✅ 纯赋值，永远合法
```

第二行那条规则很重要：**它让「每次调用都写 `err`」变得可行**，是 Go 错误处理能忍受的前提。

### 顺带：`:=` 的作用域陷阱

换一层作用域，`:=` 一定创建**新变量**，哪怕同名（变量遮蔽）：

```go
err := step1()
if cond {
    err := step2()      // ← 新变量，只活在 if 块里
    if err != nil { log.Println(err) }
}
return err              // ← 返回的是外层那个 err，错误被吞了
```

Java 不允许局部变量遮蔽外层局部变量，编译不过；**Go 编译器对此完全沉默**。写 `:=` 在 if / for 内部之前，先问一句「外面有同名的吗」。

---

## 10. 栈、堆和「逃逸分析」是什么

> 速查版。深入讲解见 D17（性能与内存）。

### 栈 vs 堆

| | 栈 | 堆 |
|---|---|---|
| 分配 | 移动一个指针，几个 CPU 指令 | 找空闲内存，慢得多 |
| 释放 | 函数返回时整帧弹掉，零成本 | 等 GC 判定 |
| 生命周期 | **不能超过当前函数** | 任意长 |

活得比创建它的函数更久的数据，**必须**在堆上——否则就是 C 里的悬垂指针。

### 三种语言的策略

- **C/C++**：你自己选，选错自己负责
- **Java**：`new` 出来的一律在堆，栈上只有引用（JVM 的逃逸分析是 JIT 干的，不可见不可控）
- **Go**：**你不选，编译器选**——这就是逃逸分析

**关键：Go 里 `new()` 和 `&T{}` 不代表堆分配。** 它们只表示「我要一个指针」，内存放哪由编译器分析后决定。

### 什么叫逃逸

> **一个变量的引用「逃出」了创建它的函数作用域。**

编译器只问一句：**这个变量的地址，函数返回后还会被用到吗？**
不会 → 栈；会 → 必须堆。

这解释了为什么 Go 里这样写是安全的：

```go
func New() *User { u := User{}; return &u }   // ✅ 编译器发现 u 逃逸，把它挪到堆上
```

同样的代码在 C 里是 UB。不是 Go 有魔法，是编译器替你搬了家。

### ⭐ 取地址 ≠ 逃逸

```go
func f() int {
	x := 42
	p := &x       // 取了地址
	return *p     // 但指针没跑出函数 → x 仍在栈上
}
```

「用了 `&` 就有 GC 压力」是错的。同理 `&User{...}` 只要没跑出函数，编译器直接放栈上，零分配。

### 常见逃逸原因

| 原因 | 例子 |
|---|---|
| 返回局部变量的指针 | `return &x` |
| 赋值给全局变量/更外层结构 | `global = append(global, &x)` |
| **传给 `interface{}`** | `fmt.Println(x)` ← 高频易忽略 |
| 闭包捕获且闭包本身逃出 | `return func() { use(x) }` |
| 编译期大小不确定 | `make([]int, n)`，n 是变量 |
| 太大（`make`/`new` 约 64KB 上限） | `make([]int, 100000)` |
| 编译器分析不出（反射、跨包） | 保守起见一律逃逸 |

### 怎么看

```bash
go build -gcflags='-m -l' ./...    # -l 关掉内联，输出更清晰
```

### ⚠️ 「does not escape」≠「在栈上」

实测（`testing.AllocsPerRun`）：

```
make([]int, 10)   分配 0 次     ← 常量大小
make([]int, n)    分配 1 次     ← 变量大小，-m 却说 "does not escape"
```

**「不逃逸」只说明生命周期没超出函数，不代表编译器有办法放栈上。** 大小编译期未知时栈帧布局不出来，仍然走堆。

**想知道有没有分配，别只看 `-m`，用 `testing.AllocsPerRun` 或 `go test -bench -benchmem` 实测。**

### 现在需要在意吗：不需要

一次分配几十纳秒，你的 HTTP 请求是毫秒级的。为「减少逃逸」扭曲代码结构是典型的过早优化，换来的是可读性下降和别名 bug。

**先测量，再优化。** 现在知道它存在、能解释「返回局部地址为什么安全」就够了。

---

## 11. Go 怎么表示集合（set）？为什么是 `map[K]struct{}`

**Go 标准库没有 set 类型。** 惯用写法是拿 map 的 key 当集合，value 用零尺寸类型占位：

```go
seen := make(map[string]struct{}, len(items))

if _, ok := seen[v]; ok { /* 已存在 */ }
seen[v] = struct{}{}          // 写入
delete(seen, v)               // 删除
len(seen)                     // 大小
```

`struct{}{}` 这个写法确实丑 —— **类型是 `struct{}`（没有字段的结构体），字面量是 `struct{}{}`**（类型 + 一对空的初始化大括号）。认了，它是惯例。

### `struct{}` 是零尺寸类型（zero-size type）

`unsafe.Sizeof(struct{}{}) == 0`。它**不占任何内存**，所有零尺寸变量共享运行时的同一个地址 `runtime.zerobase`。

> 副作用：规范说「两个不同的零尺寸变量**可能**有相同地址」——所以**别去比较它们的地址**，那是未定义行为（见 D2 §2.1.3）。

### 三种写法实测（20 万条目）

```
map[int]struct{}    4.73 MB    2.50 ms
map[int]any         6.99 MB    2.83 ms      ← +48% 内存
map[int]bool        4.73 MB    2.38 ms      ← 和 struct{} 一样大
```

**结论要分开说：**

- **对 `any`：内存和表意双输。** `any` 是接口，每个 value 占两个字长（类型指针 + 数据指针）= 16 字节。20 万个 key 就多出 2.3MB。
- **对 `bool`：当前 Go 版本内存一样**（1 字节被 map 的内部布局吸收了）。优势只在**表意**。

### 表意才是主要理由

```go
seen := make(map[T]any)      // 读者：这 value 有什么用？什么时候会存别的？
seen[v] = ""                 // 读者：为什么是空字符串？有特殊含义吗？

seen := make(map[T]bool)     // 读者：false 是什么意思？「见过但无效」？
                             //       要不要判断 if seen[v] == false？

seen := make(map[T]struct{}) // 读者：哦，set，只关心 key。
seen[v] = struct{}{}
```

**`map[K]struct{}` 在 Go 里就等于「这是个集合」的类型签名。** 零尺寸类型能装的信息量是 0，所以读者一眼就知道 value 无意义、不需要去想它。

### 什么时候反而该用 `map[K]bool`

需要**区分「不在集合里」和「在集合里但标记为 false」**时：

```go
enabled := map[string]bool{"a": true, "b": false}
if v, ok := enabled[k]; ok && v { ... }     // 两级状态，bool 是对的
```

单纯的「在不在」用 `struct{}`，需要第二个状态位才用 `bool`。

### `struct{}` 的其他用途

```go
done := make(chan struct{})    // ⭐ 只传信号不传数据的 channel（第 2 周高频）
close(done)                    // 关闭即广播
```

`chan struct{}` 是「纯信号」的标准表达，比 `chan bool` 更清楚：**接收方不该去看收到的值**。

---

## 12. 预分配的 cap 会不会白占内存？`slices.Clip`

会。预分配是对的，但结果**长期持有**时要留意：

```go
out := make([]T, 0, len(s))     // 预分配，避免多次扩容 —— 这一步没错
// ... 一千万条只留下 3 条
return out                       // len=3，但 cap=一千万，那块内存一直占着
```

和「子切片拖住整个底层数组」（D3 §4）是**同一类问题**：GC 回收的是整块底层数组，不是「你用到的那一小段」。

### 收尾：`slices.Clip`

```go
return slices.Clip(out)     // 等价于 out[:len(out):len(out)]
```

它把 cap 削到等于 len。注意 **`Clip` 本身不拷贝**，只是改 slice header——所以：

- **它并不能立刻释放内存**：底层数组仍被那个 header 指着
- 它的作用是**让后续的 `append` 必然分配新数组**，从而断开与原数组的联系；老数组在没人引用后才会被 GC 回收

想立刻释放，得真拷贝：

```go
return slices.Clone(out)    // 新分配一块刚好大小的内存，老的可以回收
```

### 什么时候做

| 场景 | 做法 |
|---|---|
| 结果是临时的，用完就丢（绝大多数） | **什么都不做** |
| 结果要长期持有，且 cap 远大于 len | `slices.Clone` |
| 要把内部切片返回给外部，防止对方 append 踩你 | `out[:len:len]` 或 `slices.Clip` |

**默认什么都不做。** 为一个临时结果多拷贝一次不划算，知道有这个选项就行。

---

## 13. 一个 nil 指针，装进 error 接口后为什么就不是 nil 了

**一句话**：`nil` 指针本身没有接口，接口是在**赋值/return 的那一刻**才产生的；装箱时编译器把「表达式的静态类型」填进接口的类型格，**不管值是不是 nil**，于是接口就非 nil 了。

### 现象

```go
type MyErr struct{}
func (e *MyErr) Error() string { return "boom" }

func f() error {
	var e *MyErr    // nil 指针
	return e
}

f() == nil          // false  ⚠️
fmt.Println(f())    // boom   ← 打印结果完全不可信，见下面「怎么调试」
```

调用方的 `if err != nil` 会成立，然后去处理一个**根本不存在的错误**。

### 装箱发生在哪一行

```go
func f() error {          // ← 返回类型是 error，这是个【接口】
	var e *MyErr          // ① e 就是个 *MyErr 指针，值 = nil。此处【没有接口】
	return e              // ② 隐式转换：装箱 ← 坑在这一行
}
```

```
        e （*MyErr 类型的变量）
        └── 值：nil
                │
                │  return e  ── 装进 error 接口
                ↓
        接口值 = ( 类型: *MyErr , 值: nil )
                    ↑              ↑
                 从【静态类型】填    从【变量的值】填
                 编译期定死，永远非空   这里是 nil
```

最能说明问题的一段——**同一个变量，比较结果相反**：

```go
var e *MyErr
fmt.Println(e == nil)        // true    ← 指针比较

var err error = e            // 装箱
fmt.Println(err == nil)      // false   ← 接口比较。e 一点没变，变的是拿什么去比
```

### 四种状态

| 表达式 | 类型格 | 值格 | `== nil` |
|---|---|---|---|
| `var e *MyErr` | —（不是接口，就一个指针） | nil | ✅ true |
| `var err error` | nil | nil | ✅ true |
| `var err error = e` | **`*MyErr`** | nil | ❌ **false** |
| `var err error = &MyErr{}` | `*MyErr` | `0x...` | ❌ false |

第 1 行和第 3 行装的是同一个 nil，结论却相反。

### 会发生隐式装箱的位置

都是同一件事，只是语法不同：

| 写法 | 装箱时机 |
|---|---|
| `return e`（返回类型是接口） | return 那一行 |
| `var err error = e` | 赋值那一行 |
| `f(e)`（形参是接口） | 传参那一刻 |
| `[]error{e}` / `map[string]error{...}` | 构造字面量时 |
| `ch <- e`（chan 元素是接口） | 发送时 |

### 为什么 Java 没有这个坑

```java
MyErr e = null;
Exception err = e;                 // err 就是 null
System.out.println(err == null);   // true
```

Java 的引用变量**只存一个地址**，null 就是 null，赋给谁都还是 null——类型信息长在对象自己身上，对象不存在就没有类型信息。

Go 的接口值是**两个字长**（类型描述符 + 数据指针），类型格由编译器按静态类型填死，**哪怕数据格是 nil 也照填**。所以「空指针」和「空接口」在 Go 里是两个不同的状态。

TS 也没有这个坑，因为 TS 的接口是编译期擦除的，运行时根本不存在。**Go 的接口在运行时是有实体的**，这是根因。

### 两条避免规则

**① 函数返回值类型写 `error`，别写具体错误类型。**

```go
func Validate(r Record) *ValidationError   // ❌ 埋雷：调用方一转成 error 就中招
func Validate(r Record) error              // ✅
```

**② 中间变量是具体类型时，别写 `return f()`，先判断再 return。**

```go
// ❌
func Save(r Record) error {
	return Validate(r)          // 合法时返回的也不是 nil
}

// ✅
func Save(r Record) error {
	if verr := Validate(r); verr != nil {   // 这里是【指针比较】，正确
		return verr                          // 只在真非 nil 时才装箱
	}
	return nil                               // 显式返回 nil 接口 (nil, nil)
}
```

> 变量名别叫 `err`。叫 `verr` 或 `vErr`，提醒读者「它是具体类型，不是 error」。

### 怎么调试

**先破除一个误解**（我第一版写错过）：typed nil 的 error 打印出来**不一定**是 `<nil>`。`fmt` 遇到 error 会**调用它的 `Error()` 方法**，结果取决于这个方法碰没碰接收者——实测：

```go
type A struct{ Code int }
func (e *A) Error() string { return "boom" }                          // 不碰接收者

type B struct{ Code int }
func (e *B) Error() string { return fmt.Sprintf("code=%d", e.Code) }  // 解引用了
```

```
var errA error = (*A)(nil)   →  fmt.Println(errA) 打出  boom     ← 方法正常返回了
var errB error = (*B)(nil)   →  fmt.Println(errB) 打出  <nil>    ← 方法 panic，被 fmt 兜住了
```

`errB` 那个 `<nil>` 是 `fmt` 内部的兜底：`Error()` 在 nil 接收者上解引用会 panic，`fmt` 捕获后发现实参是 nil 指针，于是打了 `<nil>`。**它不表示「值是 nil」。**

所以日志里你可能看到 `boom`、`<nil>`、或 `%!v(PANIC=...)`——**三种都不告诉你这个 error 其实非 nil**。

**唯一可信的是 `%T` 和 `== nil`：**

```go
fmt.Printf("%v %T %v\n", err, err, err == nil)
// boom *main.A false      ← 只有中间那格能说明问题
```

### 工具能兜住一部分

`golangci-lint`（staticcheck）会报：

```
SA4023: f never returns a nil interface value
SA4023: this comparison is never true
```

但**只在它能静态推导出「永远非 nil」时才报**。中间隔几层函数调用、或者具体类型来自另一个包时就看不出来了。规则还是要记在脑子里。

### ⚠️ 二阶陷阱：`errors.As` 会返回 true，然后给你一个 nil

实测过：

```go
var v *VErr
var err error = v          // typed nil

var target *VErr
errors.As(err, &target)    // → true！target 被赋成了 nil
target.Field               // → panic: nil pointer dereference
```

`errors.As` 只按**类型**匹配，类型是对得上的。所以「`errors.As` 返回 true」不等于「拿到了可用的对象」。这进一步说明：**typed nil 必须在源头掐掉**，别指望下游的错误处理帮你兜。

---

## 14. 跑 benchmark 时为什么要加 -run 参数

**一句话**：`-bench` 只筛选**基准测试**，`-run` 筛选**普通测试**。不写 `-run` 时它默认是 `-run=.`（匹配全部），于是包里所有 `TestXxx` 都会跟着跑一遍。`-run='^$'` 是个匹配不到任何测试名的正则，效果就是「一个测试都不跑，只跑 benchmark」。

```bash
go test -bench=. -benchmem -run='^$' ./internal/genx/
```

### 三个理由

**① 最要紧：只要有一个测试失败，基准测试压根不会跑。**

`go test` 的顺序是**先跑测试，全过了才跑 benchmark**。实测对比（包里放了一个失败的测试和一个 sleep 800ms 的慢测试）：

```
######## 不加 -run
--- FAIL: TestFailing
FAIL                              ← ⚠️ BenchmarkSum 一行都没有
用时 2.78s

######## 加 -run='^$'
BenchmarkSum-10   3006553   397.9 ns/op   0 B/op   0 allocs/op
ok
用时 1.64s
```

**而且报错里完全看不出「你的 benchmark 被跳过了」。** 调优时这个尤其烦：你正在改实现、测试暂时是红的，然后怎么都看不到性能数据。

**② 慢测试白白拖时间。** 这里差 1.1 秒；真实项目里起容器、连数据库的集成测试能让你每次调优都等半分钟。

**③ 输出混杂。** benchmark 结果本来就要人眼比对，中间夹一堆 `=== RUN` 很难读。

### 为什么写 `^$`

`^$` = 锚定的空字符串，没有任何测试名能匹配。也见过有人写 `-run=XXX` / `-run=NONE`——效果一样，但靠的是「赌没有测试叫这个名字」。**`^$` 表达的是「我就是要一个都不跑」**，意图明确。

### 反过来：不加 `-bench` 时 benchmark 不会跑

这是默认行为，所以日常 `go test ./...` 不会被 benchmark 拖慢。四个前缀的执行时机见 `lessons/D6.md` §2：

| 前缀 | 什么时候跑 |
|---|---|
| `TestXxx` | 默认跑 |
| `BenchmarkXxx` | **只有加 `-bench` 才跑** |
| `ExampleXxx` | 默认跑（前提是有 `// Output:`） |
| `FuzzXxx` | 默认只跑种子；加 `-fuzz` 才真正模糊 |

### 常用组合速查

```bash
go test -bench=. -benchmem -run='^$' ./pkg/          # 标准姿势
go test -bench=BenchmarkFilter -benchmem -run='^$' ./pkg/   # 只跑某一个
go test -bench=. -benchtime=10s -run='^$' ./pkg/     # 每个跑够 10 秒（默认 1 秒）
go test -bench=. -benchtime=100x -run='^$' ./pkg/    # 固定跑 100 次
go test -bench=. -count=10 -run='^$' ./pkg/ > new.txt
benchstat old.txt new.txt                            # 统计显著性对比
```

**`-benchmem` 一定要加**——`allocs/op` 往往比 `ns/op` 更能说明问题，而且跨机器可比。三个指标的含义见 `lessons/D6.md` §6。

---

## 15. 无缓冲 channel 什么时候会死锁

**一句话**：给每个阻塞的 goroutine 画一条箭头指向「谁能解救我」，**箭头成环就是死锁**。和「发送还是接收」无关。

### 先破除一个错误的记法

「所有参与者都在发送这一侧就会死锁」——**这是特例，不是规律**。两个都在接收照样死锁：

```go
// A：互相【发送】→ 死锁
x, y := make(chan int), make(chan int)
go func() { x <- 1; <-y }()
y <- 2
<-x
// fatal error: all goroutines are asleep - deadlock!
// goroutine 1 [chan send] / goroutine 34 [chan send]

// B：互相【接收】→ 也死锁
go func() { <-y; x <- 1 }()
<-x
y <- 2
// goroutine 1 [chan receive] / goroutine 6 [chan receive]

// C：一发一收，顺序对上了 → 正常
go func() { <-x; y <- 2 }()
x <- 1
fmt.Println(<-y)     // OK 2
```

### 正确的模型：等待图

无缓冲 channel 的本质是**交接**——收发必须同时到场。所以每个阻塞的 goroutine 都在等一个具体的对象：

| 阻塞在 | 它在等 |
|---|---|
| `ch <- v` | **有人来 `<-ch`** |
| `<-ch` | **有人来 `ch <- v`** |

顺着「谁能解救我」走一圈，**回到起点就是死锁**。C 能跑是因为箭头是一条链而不是环。

### 实战：worker pool 的经典死锁

```go
jobs := make(chan Job)
results := make(chan Result)      // 都无缓冲

// N 个 worker：for j := range jobs { results <- process(j) }

for _, j := range allJobs { jobs <- j }   // ⚠️ 主流程同步投递
close(jobs)
for r := range results { ... }            // 投递完才开始收
```

```
主流程 卡在 jobs <- j     ──等待─→ 有 worker 来取 jobs
                                      ↓ 但 worker 全卡在发结果
worker 卡在 results <- x  ──等待─→ 有人来取 results
                                      ↓ 只有主流程会取
                                   主流程 ← 环闭合
```

**主流程既是 `jobs` 的唯一发送者，又是 `results` 的唯一接收者**——它把自己卡在第一个角色上，第二个角色就没人干了。

⭐ **由此得到一条实用判据：一个 goroutine 不能既是某条 channel 的唯一接收者，又在别处阻塞着。**

### 两种破环方式

| 方式 | 原理 | 代价 |
|---|---|---|
| 给 `results` 加缓冲 `len(jobs)` | 缓冲位是「不需要对方到场就能放下的槽」，前 N 次发送**不产生等待边** | 内存 O(N) |
| **把投递挪进 goroutine** ⭐ | 主流程**卸掉一个角色**，只负责收集，永远随时可收 | 内存 O(1) |

```go
go func() {
    for _, j := range allJobs { jobs <- j }
    close(jobs)
}()
for r := range results { ... }    // 边投边收
```

**关键不是「多起了一条 goroutine」，是「主流程卸掉了一个角色」。**

### code review 时怎么查

1. 列出所有**无缓冲**的 channel 操作（`make(chan T)` 不带第二个参数）
2. 对每条 goroutine，写下它的阻塞点顺序
3. 顺着「谁能解救我」走一圈，看会不会回到起点

最容易漏的是第 3 步里**「唯一接收者」这个隐含前提**——你以为有人会来收，其实那个人正卡在别处。

### ⚠️ 死锁检测只在「全部睡着」时触发

```
fatal error: all goroutines are asleep - deadlock!
```

只要还有**一条** goroutine 在跑（HTTP server 的 accept 循环、后台定时器、一个空转的循环），**这个环就静默地卡在那儿**，运行时不会报任何东西。

真实服务里的表现是：**某个请求永远不返回** + **goroutine 数只增不减**。那时候要靠 `/debug/pprof/goroutine` 看栈（D16）——栈上的 `[chan send]` / `[chan receive]` 标记会直接告诉你这条 goroutine 卡在等什么、卡在哪一行。


---

## 16. `SetWriteDeadline`：两次写之间隔很久，会不会超时

**一句话**：不会。写 deadline 限的是**单次写操作最多能阻塞多久**，不是「连接能活多久」，也不是「两次写之间能隔多久」。deadline 到期时如果没有正在进行的写操作，它**什么都不做**——不报错，也不关连接。

背景：D11 的 SSE 示例 `events.go` 里，每次写之前都调一遍 `rc.SetWriteDeadline(time.Now().Add(window))`。疑问是——这次写完，到下次有数据可写之间过了很久，deadline 早过期了，会怎样？

### 实测（`window=200ms`，两次写之间空闲 `1s`，是窗口的 5 倍）

| handler 的写法 | 客户端收到 | handler 里的错误 |
|---|---|---|
| ① 每次写前都重设（`events.go` 的做法） | `[first second]` | `<nil>` |
| ② 只在开头设一次，之后不重设 | `[first]` | `write tcp ...: i/o timeout` |
| ③ 重设成**零值**（清除超时） | `[first second]` | `<nil>` |

再压一档：`window=50ms`、写间隔 `400ms`（窗口的 8 倍），连写 5 次全部成功。

### ⚠️ 真正的坑：过期状态是「粘住」的

情况 ② 失败得很有意思。量一下那次写的耗时：

```
第二次写耗时 = 121.875µs，错误 = write tcp ...: i/o timeout
```

**122 微秒**——它根本没阻塞过，是**立刻**失败的。deadline 过期后 fd 被标记成「已超时」，这个状态**一直粘着**，直到下一次 `SetWriteDeadline` 把它清掉。

所以规则不是「窗口要设得够大」，而是：

> ⭐ **漏掉任何一次 `SetWriteDeadline`，这条连接就当场作废。**

这也是为什么 `events.go` 把「设 deadline + 写 + flush」绑成一个 `writeAndFlush` 函数——让你没机会漏。

### 由此更正了一条早先的说法

`events.go` 和 D11 笔记里原先写着：

> ~~`writeWindow` 必须**大于** `heartbeat`，否则心跳还没来得及发，连接就被砍了~~

**这是错的**，上面 `window=50ms / 间隔 400ms` 的测试直接推翻了它。两者谁大谁小无所谓，因为每次写之前都重设了。当时是把**写超时**当成了**空闲超时**——管空闲的是 `http.Server.IdleTimeout`，是另一个东西。

### 那零值（永不超时）行不行

语法上行（情况 ③），但 SSE 里**不要用**：零值意味着一个卡死的客户端能让这条 goroutine 和连接**永远**挂着。给一个大但有限的窗口（`events.go` 里是 30s），配合心跳，才是对的。

---

## 17. `WriteTimeout` 和 `SetWriteDeadline` 同时存在时谁说了算

**一句话**：`SetWriteDeadline` **取代** `WriteTimeout`。它们不是两套机制，是同一个东西——一条连接上只有**一个**写 deadline，谁最后调谁说了算。不取较小值，也不叠加。

### 实测（`http.Server.WriteTimeout = 300ms`）

| handler 里做的事 | 结果 |
|---|---|
| ① 什么都不做，慢 600ms | ⚠️ `i/o timeout` |
| ② `SetWriteDeadline(+2s)`，慢 600ms | ✅ 成功 —— **放宽生效** |
| ③ `SetWriteDeadline(+100ms)`，慢 200ms | ⚠️ `i/o timeout` —— **收紧也生效** |
| ④ `SetWriteDeadline(零值)`，慢 600ms | ✅ 成功 —— **彻底关掉** |

③ 最能说明问题：服务器配的是 300ms，我在 handler 里设成 100ms，200ms 就被砍了——**比服务器配置更严**。

### 机制：`WriteTimeout` 只是替你调了同一个方法

`net/http/server.go` 的 `readRequest` 里：

```go
if d := c.server.WriteTimeout; d > 0 {
	defer func() {
		c.rwc.SetWriteDeadline(time.Now().Add(d))   // ← 就是同一个方法
	}()
}
```

`http.ResponseController.SetWriteDeadline` 最终也是调 `c.rwc.SetWriteDeadline`。所以「后写的覆盖先写的」。

⚠️ 注意那个 `defer`：deadline 是在**请求头读完之后**才装上的，起点**不是**连接建立时——这就是 D11 §8 那个坑：`WriteTimeout` 实际覆盖的是「读完请求头 → 响应写完」一整段，**包含你 handler 里的业务耗时**。

### 覆盖只在**当前请求**内有效

keep-alive 复用同一条连接的实测：

```
请求 1  conn=127.0.0.1:62284  放宽到 +5s     ✅ 写成功
请求 2  conn=127.0.0.1:62284  没做任何设置    ⚠️ 写失败 (i/o timeout)
→ 两个请求确实走的同一条 TCP 连接
```

因为上面那段代码在 `readRequest` 里，**每读一个新请求就重新装一次**。第 1 个请求把 deadline 推到 +5s，第 2 个请求一进来就被重置回 `now+300ms`。

所以不用担心「某个 SSE handler 放宽了 deadline，会把后面复用这条连接的普通请求也带松」——不会。

### 实践结论

> ⭐ `WriteTimeout` 设成对**普通接口**合理的值（比如 10s），长连接接口（SSE、下载）在自己的 handler 里用 `SetWriteDeadline` **局部豁免**。
>
> 不要为了迁就一个 SSE 接口，把全局 `WriteTimeout` 调大或干脆设成 0 ——那等于所有接口都失去保护。

（相关：[#16](#16-setwritedeadline两次写之间隔很久会不会超时)、D11 §8 四个超时）


---

## 18. 包装 `slog.Handler` 为什么会丢字段

**一句话**：`slog.Handler` 有**四个**方法，其中 `WithAttrs` / `WithGroup` **返回 Handler 自身**。只重写 `Handle` 的话，一调 `.With(...)` 包装层就被剥掉了——而且**没有任何征兆**。

### 场景

给日志自动带上 ctx 里的 request ID，标准写法是包一层 Handler：

```go
type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := RequestIDFrom(ctx); ok {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}
```

### 实测：`.With()` 之后 `request_id` 消失

```
--- A：只重写 Handle ---
{"msg":"直接记","request_id":"req-42"}                  ✅
{"msg":"先 With 再记","user":"alice"}                    ⚠️ 没了
{"msg":"先 WithGroup 再记"}                              ⚠️ 没了

--- B：三个都重写 ---
{"msg":"直接记","request_id":"req-42"}                  ✅
{"msg":"先 With 再记","user":"alice","request_id":"req-42"}      ✅
{"msg":"先 WithGroup 再记","http":{"request_id":"req-42"}}       ✅
```

### 为什么

```go
type Handler interface {
	Enabled(context.Context, Level) bool
	Handle(context.Context, Record) error
	WithAttrs([]Attr) Handler        // ← 返回 Handler
	WithGroup(string) Handler        // ← 返回 Handler
}
```

嵌入 `slog.Handler` 后，`WithAttrs`/`WithGroup` 被自动提升——但它们是**内层 handler 的**方法，返回的也是**内层那个没被包装的** handler：

```
slog.New(ctxHandler{base})       handler = ctxHandler{base}    ✅
    .With("user", "alice")       handler = base                ❌ 包装脱落
```

⚠️ 而 `logger.With(...)` 正是「给每个请求做日志上下文」的标准手法（讲义 D12 §3），**最推荐的用法恰好触发这个 bug**。

### 修法

把两个返回 `Handler` 的方法重写，结果重新包一层：

```go
func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(name)}
}
```

`Enabled` **不用重写**——它返回 `bool`，不涉及包装层。

### ⭐ 一条可复用的判据

> 包装一个接口时，**逐个看它的方法签名：凡是返回该接口自身类型的，都必须重写并把结果重新包一层**，否则包装层会在第一次调用后脱落。

对照几个常见接口：

| 接口 | 返回自身类型的方法 | 要小心吗 |
|---|---|---|
| `slog.Handler` | `WithAttrs`、`WithGroup` | ⚠️ 要 |
| `http.ResponseWriter` | 无 | 不用（但有另一种漏法，见下） |
| `io.Reader` / `io.Writer` | 无 | 不用 |
| `context.Context` | 无 | 不用 |

### 和 D11 那个坑的关系

这和 D11 §5 的 `responseRecorder` 是**同一类问题**：嵌入接口 + 只重写关心的方法。但漏的方式不同：

| | D11 `responseRecorder` | 这里 `ctxHandler` |
|---|---|---|
| 漏什么 | **可选接口**（Flusher/Hijacker…）没被提升 | **包装层**被 `With*` 剥掉 |
| 怎么发现 | 类型断言失败，SSE 不刷新 | **没有征兆**，日志里静默少一个字段 |
| 修法 | 加 `Unwrap()`，用 `ResponseController` | 重写 `WithAttrs`/`WithGroup` |

**第二行是重点**：D11 那次至少还有个可观察的故障；这次只是日志少了个字段，线上排查时你甚至不会想到是包装层的问题。

（相关：D11 §5 嵌入接口的代价、D12 §3 从 ctx 提取字段）


---

## 19. 非导出类型上的大写方法，算导出吗

**一句话**：不算。**大写只是「可导出」的必要条件，不是充分条件**——方法能不能被外部访问，还要看它所属的类型能不能被外部**命名**。

### 场景

包装 `slog.Handler` 时（见 [#18](#18-包装-sloghandler-为什么会丢字段)）会写出这么一坨：

```go
type ctxHandler struct{ slog.Handler }                              // 小写

func (h ctxHandler) Handle(...) error                               // 大写
func (h ctxHandler) WithAttrs(...) slog.Handler                     // 大写
func (h ctxHandler) WithGroup(...) slog.Handler                     // 大写

func WithRequestID(h slog.Handler) slog.Handler { return ctxHandler{h} }   // 导出的入口
```

看着别扭：类型藏起来了，方法却全大写，像是把三个方法暴露了出去。

**首先，方法名没得选**：`slog.Handler` 接口声明的就是 `Handle`/`WithAttrs`/`WithGroup`，写成小写就不满足接口。

### 实测：外部到底能做什么

拿一个等价的小例子（非导出 `greeter` + 导出接口 `Greeter` + 构造函数 `New`）：

| 外部包想做的事 | 结果 |
|---|---|
| 通过接口调 `g.Greet()` | ✅ `hello, world` |
| 直接写 `inner.greeter` | ❌ `name greeter not exported by package inner` |
| `go doc` 里能看到 | ❌ 只列出 `Greeter` 和 `New` |

```
$ go doc ./inner
package inner // import "ex/inner"

type Greeter interface{ ... }
    func New(name string) Greeter
```

`greeter` 和它的 `Greet` **一个字都不出现**。

⭐ **`go doc` 的输出就是「这个包对外承诺了什么」的权威度量。** 拿不准某个东西算不算公开 API，跑一下 `go doc` 最直接。

### 为什么

> **Go 的导出规则按标识符逐个判定，但可达性是「链式」的。**

`ctxHandler.Handle` 这个标识符是大写的，可它前面那截 `ctxHandler` 是小写的——整条路在第一段就断了。外部连类型名都写不出来，自然拿不到它的方法；唯一通路是你导出的接口和构造函数。

### 这是标准库的常规做法

```go
// errors 包
type errorString struct{ s string }            // 非导出
func (e *errorString) Error() string { ... }   // 大写 —— error 接口要求的
func New(text string) error { ... }            // 导出的构造函数
```

`context` 包同理：`WithCancel` 返回的 `*cancelCtx` 是非导出的，但 `Deadline`/`Done`/`Err`/`Value` 全大写，因为 `context.Context` 接口这么要求。

**这个组合恰恰是收窄 API 的手段**：外部只知道「有个 `slog.Handler`」，不知道内部叫什么、什么结构。以后你想给 `ctxHandler` 改名、加字段、甚至换成完全不同的实现，都不算破坏兼容——**它从来就不在公开面上**。

### ⚠️ 反过来才要小心

在**导出的**类型上加大写方法，那是实打实扩大了 API 面：

```go
type Store struct{ ... }                 // 导出
func (s *Store) Debug() string { ... }   // ⚠️ 这个是真的公开了，以后删不掉
```

**判据**：加方法前先看接收者类型是大写还是小写。小写随便加，大写要当成对外承诺。

（相关：[#18](#18-包装-sloghandler-为什么会丢字段)、[#1](#1-cmd-目录是-go-的命名规范吗) 工程约定）
