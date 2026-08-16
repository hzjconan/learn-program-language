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
