# D17 ② 优化 `internal/logx`

> 返回 [D17 笔记](../D17.md) · 讲义 [lessons/D17.md](../../lessons/D17.md) §4、§5

（2026-10-15 → 10-16。以 `BenchmarkBig_EndToEnd` 为准：20 万行，解析一遍 + 出一份文本报告）

## 结果

| | 基线 | 最终 | 变化 | 目标 |
|---|---|---|---|---|
| 耗时 | 105.75ms | **42.65ms** | **−59.7%（2.48 倍）** | ≥ 2 倍 ✅ |
| 分配次数 | 601.8k | **202.4k** | **−66.4%** | 降到一半以下 ✅ |
| 分配字节 | 136.0MiB | 91.3MiB | −32.9% | — |

`TestBigReport_Golden`（文本 + JSON）和原有测试全程全绿，公开 API 签名没变。

## 每一步（benchstat，`-count=10`，p 全部 = 0.000）

| 步 | 改了什么 | 耗时 | 分配次数 |
|---|---|---|---|
| 1 | `TopSlowest`：全量复制 + 排序 20 万条 → 遍历一遍，只维护前 N 名的有序小切片 | 105.75 → 72.45ms，**−31.5%** | 不变 |
| 2 | `Analyze`：`scanner.Text()` 调了两次 → 一次 | 72.45 → 63.25ms，−12.7% | 601.8k → 402.2k |
| 3 | `ParseLine`：`strings.SplitN` → 连续 4 次 `strings.Cut` | 63.25 → 55.88ms，−11.7% | 402.2k → 202.4k |
| 4 | `Percentile`：`Format` 调 3 次、每次复制 + 排序 → 不导出的 `percentiles` 一次排序算出 3 个 | 55.88 → 42.65ms，**−23.7%** | 不变（每轮少 2 次大分配） |

⚠️ 第 2 步的 step1 → step2 还混进了 `TopSlowest` 的重构（按 review 改成单一比较函数）。那份 profile 里 `TopSlowest` 只剩 2.7%，影响可以忽略，但严格说违反了「一次只改一处」。

## 从 profile 读出的判断

基线 profile（我一开始的判断有三处错，见最后）：

- **CPU**：两处排序加起来约一半。`TopSlowest` cum 32.7%，`Percentile` cum 16.8%（用 `-top -cum` 确认，不能只看 flat）
- **分配**：每行 3 次 = `scanner.Text()` 两次 + `SplitN` 一次。`Text` 355 万次 ≈ `genSplit` 159 万次的两倍多
- `bigLog` 是生成测试数据的函数，只在包初始化时跑一次（`runtime.doInit`），不是被测代码

第 1 步之后：`runtime.madvise` 涨到 39.6%。查调用栈，来自 `mallocgc`、GC 标记、`slicebytetostring`，根源是**分配太多**。
macOS 上「还给系统」和「重新拿回来用」都要调 `madvise`（`runtime/mem_darwin.go`），Linux 上重新使用不需要主动调，所以本机看到的比例有一部分是 macOS 造成的。以 benchstat 的 `sec/op` 为准。

## 学到的

- **算法优先**：第 1、4 步改的是「做了多少不必要的排序」，两步合计贡献了大头。常数级的优化（第 2、3 步）各一成多
- **分配少了，收益比 profile 显示的更大**：GC、`madvise` 的开销算在 runtime 名下，不在业务函数名下
- **规则只能有一份**：排名规则只留 `compareEntry`；分位数下标公式只留 `percentileOfSorted`；「复制 + 排序」只留在 `percentiles` 里，公开的 `Percentile` 调它
- `result = result[1:]` 会让容量少 1，接着 `Insert` 就要重新分配；从尾部截断（`result[:n]`）不会
- **`Text()` 那一次分配省不掉**：`Bytes()` 返回的内存下一次 `Scan()` 会被覆盖，而 `Entry.Service`、`Entry.Message` 是这一行的子串、会一直存在 `Report` 里。别人借给你、之后会复用的内存，不能留着引用；要留就复制。和 D16 ⑤ 的 `buf[:16]`、§6.2 `sync.Pool` 第 3 个坑同一类。`unsafe.String` 「零拷贝」在这里恰恰会触发这个 bug
- `Cut` 返回的子串和 `s` 共享内存并被存进 `Entry`，安全的原因是 `s` 是不可变的 string，不是「只是读」

**缓存方案的并发问题**（讲义练习 ② 的问题）：在 `Report` 里缓存排好序的结果，第一次调用 `Percentile` 时写入，等于把一个只读方法变成了**会写共享状态**的方法。两个 goroutine 同时对同一个 Report 调 `Format`，就会同时写这个缓存，数据竞争；`Add` 之后缓存还要失效。所以选了「不导出的方法，一次排序」：`Report` 里不存额外状态。
另外，`BenchmarkBig_Format` 每轮复用同一个 Report，加缓存后只有第一轮真算，会显示「快了很多」，但 `cmd/loganalyze` 每次只 Format 一次，享受不到（讲义 §4.4）。

## 我判断错的地方

| | 我的判断 | 实际 |
|---|---|---|
| CPU 热点 | `TopSlowest` 占 10.28% | 那是比较函数 `TopSlowest.func1` 的 **flat**。按 cum，`TopSlowest` 32.7%、`Percentile` 16.8% |
| 分配来源 | `bigLog`；第 3 次是 `rpt.Add` | `bigLog` 是测试数据生成；第 3 次是 `SplitN`。`append` 成倍扩容，20 万条只扩二十来次 |
| 先改什么 | 先改分配，防止 OOM | 这不是 OOM：每轮都会释放，代价是 CPU。按 profile 最大的一块（排序）决定先改哪里 |
| 第 4 步预测 | 4%（12% ÷ 3） | **23.7%**。① 3 次变 1 次省的是 2/3，不是 1/3 ② 主要漏了：每次排序前还要**复制** 20 万个耗时（1.6MB），省 2 次就少分配 3.2MB（`B/op` 正好 −3.23%），连带 GC 开销。另外 12.6% 是两步之前的占比，之后总耗时变小，占比变大，预测要用最新的 profile |
| 第 2、3 步预测 | 第 3 步预测少 20 万次分配 ✅ | 实际少 19.98 万次（空行早就 continue 了）。第 2 步没写预测 |
