# D17 ④ `sync.Pool`

> 返回 [D17 笔记](../D17.md) · 讲义 [lessons/D17.md](../../lessons/D17.md) §6

（2026-10-17。`cmd/leakdemo/server.go` 的 `ingest`：每个请求 `make([]byte, 64KB)` → `sync.Pool`）

## 结果（`BenchmarkIngest`，`-count=10`）

| | 基线 | Pool | 变化 |
|---|---|---|---|
| 耗时 | 8.53µs ± 24% | **2.21µs ± 1%** | −74% |
| 分配字节 | 69.3KiB | **5.4KiB** | **−92%** |
| 分配次数 | 19 | 18 | −1 |

- 预测 `B/op` 降到 4K 左右 ✅（剩下 5.4KiB 是 `httptest` 的请求/响应、`bytes.Clone`、map 写入）
- 预测「allocs 少 n 次，n = 请求数」：意思对，口径要换成**每次请求**：`allocs/op` 是每次操作几次，19 → 18。总量上 n 个请求少 n 次
- 基线时我预测「至少 64K」✅。19 次分配里那块 64KB 只占 1 次，**字节数却占 92%**。决定 GC 频率的是字节数，只看 `allocs/op` 会以为它不值得优化

## 改法

```go
var bufPool = sync.Pool{                       // ⭐ 包级变量：所有请求共用同一个
	New: func() any { b := make([]byte, maxIngestBytes); return &b },
}

func (s *server) ingest(w http.ResponseWriter, r *http.Request) {
	bufP := bufPool.Get().(*[]byte) //nolint:errcheck // bufPool.New 只返回 *[]byte，没有别的地方往里 Put
	defer bufPool.Put(bufP)
	buf := *bufP
	...
```

lint 开了 `check-type-assertions`，类型断言要么 comma-ok 检查，要么 `nolint` 写明为什么不会失败。

## 第一版的 bug：Pool 写在了函数里

```go
func (s *server) ingest(...) {
	var bufPool = sync.Pool{...}   // 每个请求一个全新的空 Pool
```

空 Pool 每次 `Get` 都调 `New` → 每个请求照样 `make` 64KB；`Put` 回去的随函数返回一起扔掉。

| | 基线 | 第一版 |
|---|---|---|
| 耗时 | 8.82µs ± 27% | 4.40µs（**−50%**） |
| 分配字节 | 69.35KiB | 70.80KiB（**+2%**） |
| 分配次数 | 19 | 22（**+3**：Pool 本身 + New 闭包每次都要建） |

⭐ **只看耗时会以为优化成功了。** 分配没减少、耗时却减半，本身就说不通，是基线测量被干扰了。`B/op`、`allocs/op` 不受机器负载影响，老老实实说明了「Pool 没起作用」（D6：`allocs/op` 最可靠）

## 四个坑：要拿规则去检查这段代码

我第一次的回答是复述讲义的规则。逐条对照 `ingest`：

| 坑 | 这段代码 |
|---|---|
| ① 放 `*[]byte` 还是 `[]byte` | `*[]byte` ✅。放切片要装箱进 `any`，又多一次分配（SA6002） |
| ② 放回前要不要清理 | **不需要**。`io.ReadFull` 从下标 0 **覆盖写**，后面只读 `buf[:sigLen]`，而 `n < sigLen` 就返回的检查保证了 `sigLen <= n`，读到的一定是这一次写入的 |
| ③ Put 之后还有没有引用 | 没有。`buf[:sigLen]` 传给 `dedup.add`，`add` 入口先 `bytes.Clone`，存进 `order` 的是副本。**靠的是 D16 那行 Clone** |
| ④ 会不会撑大后一直占着 | 不会。固定 64KB，`ReadFull` 只覆盖写、`LimitReader` 最多读 64KB，不扩容。「越变越大」针对的是 `bytes.Buffer` 这类**追加写**的对象 |

- 我问「同一个 buf 反复写，不会扩容吗」：`io.ReadFull(r, buf)` 往**现有空间**里填，像 Java 的 `InputStream.read(byte[])`；会扩容的是 `append`，像 `ByteArrayOutputStream`。
  小实验：8 字节 buf 依次读 12 个 A、3 个 B、5 个 C，`len/cap` 始终 8/8，内容 `AAAAAAAA` → `BBBAAAAA` → `CCCCCAAA`；`append` 则 8 → 16 → 32
- 我又追问「`buf[:sigLen]` 是定长的，不会读到旧数据吗」：Claude 之前说「代码只用 `buf[:n]`」**不准确**。真正保证安全的是 `n < sigLen` 那道检查
- ⚠️ **「不用清理」依赖那道检查**。去掉它，3 字节的请求会让 `buf[:16]` 混进上一个请求残留的 13 字节，签名错了、可能被误判重复，而且不报错。
  用 `make` 时新内存全是 0，没有这个问题；**改用 Pool 后，原来无害的代码可能变得有害**
- 真实服务里多个请求同时处理、每个 P 有自己的缓存，Pool 里实际有**若干块**缓冲区（大致等于并发请求数），各自固定 64KB

## 思考题：没有 `bytes.Clone`，用了 Pool 会怎样

我的答案：`order` 里的切片都指向 Pool 里同一块缓冲区，所有签名的内容都会变成最后一次请求的数据。✅ 但只说了一半，还要推「行为会出什么错、什么时候暴露」：

- **前 5 万条看起来完全正常**：查重用的是 `set`，key 是 `string(sig)`，存入时**复制**了一份，是正确的。`order` 在这个阶段没人读
- **第 50001 条起悄悄出错**：淘汰时 `delete(d.set, string(d.order[d.next]))`，`order[next]` 指向的缓冲区已经是当前请求的数据
  - 该删的老签名没删 → `set` 超过 5 万条上限一直涨 → **换了个方式的内存泄漏**
  - 删掉的是当前或最近请求的签名 → 同一签名再来时找不到 → **去重失效**
- 至少 5 万个不同签名才暴露，单测、本地调试都到不了；不报错，只是偶尔漏掉重复上报 + 内存慢慢涨

⭐ **同样少了一行 `bytes.Clone`：没有 Pool 时是内存泄漏（每条钉住 64KB，涨得飞快、容易发现）；有 Pool 时变成数据悄悄出错（内存不涨了，更隐蔽）。**
优化改变了 bug 的表现形式。D16 的 `TestDedupAdd_CopySig` 里「改掉原始 buffer，签名不能跟着变」那条断言正好能抓住它

## 基线的「前慢后快」

基线 `± 27%`，原始数据分成两组：

```
9648 9416 9471 9486 9910 | 8494 | 6430 6177 6180 6324
```

- 能稳定重现；每个新进程都从慢的那档开始；按**时间**发生（每轮 0.5s 或 1s，都是启动后约 5～6 秒变快），不是按轮数
- **Pool 版本完全没有这个现象**（10 次都是 2.18～2.25µs，± 1%）。Claude 先猜的是 macOS 调度或电源管理，这个对照把它推翻了：和**频繁分配 64KB**（>32KB 的大对象走另一条分配路径，加上 macOS 的 `madvise`）有关，具体哪一步没有验证
- 处理：这次以 `B/op`、`allocs/op` 为主；耗时在相同 `-count` 下比较基本公平。更严谨的是 `go test -c` 编出新旧两个测试程序**交替运行**
- ⭐ **`±` 很大时先看原始数据的分布**：benchstat 只给中位数和范围，看不出是随机散开还是分成了两组
- 只 stash 一个文件：`git stash push -- cmd/leakdemo/server.go`（未跟踪的 `server_bench_test.go` 不受影响）
