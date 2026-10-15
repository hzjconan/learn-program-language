# D16 ⑤ 靶场：两个泄漏

> 返回 [D16 笔记](../D16.md) · 讲义 [lessons/D16.md](../../lessons/D16.md) §8

1. 证据：

**goroutine 泄漏**（2026-09-29 那轮压测）

`debug=1` 按栈分组，第一组 741 个，停在 `helpers.go:20`：

```
goroutine profile: total 748
741 @ 0x1042ea91c 0x10427da98 0x10427d6a8 0x1044daed8 0x1042f22b4
#       0x1044daed7     main.withTimeout[...].func1+0x37        .../go/cmd/leakdemo/helpers.go:20
```

另外两组是正常的：2 个是两个 http.Server 在 `Accept`（`main.go:80`），1 个是这次 curl 请求自己（`runtime/pprof.writeGoroutine`）。

Go 1.26 的 `goroutineleak` profile 判定的也正好是这 741 个，位置相同：

```
goroutineleak profile: total 741
741 @ 0x1042ea91c 0x10427da98 0x10427d6a8 0x1044daed8 0x1042f22b4
#       0x1044daed7     main.withTimeout[...].func1+0x37        .../go/cmd/leakdemo/helpers.go:20
```

**内存泄漏**

`-base` 只看压测期间新增：37.23MB，100% 来自 `main.(*server).ingest`，flat = cum（是它自己分配的）。

`-list 'ingest'` 定位到 `server.go:71`：

```
ROUTINE ======================== main.(*server).ingest in .../go/cmd/leakdemo/server.go
   37.23MB    37.23MB (flat, cum) 84.47% of Total
         .          .     70:func (s *server) ingest(w http.ResponseWriter, r *http.Request) {
   37.23MB    37.23MB     71:   buf := make([]byte, maxIngestBytes)
```

`maxIngestBytes = 64 << 10 = 65536` 字节 = 64KB。
手算：1500 个 `/ingest`（202 的数量）× 64KB = **93.75MB**，但 profile 只有 37.23MB，对不上。

两种解释：① 只留住了一部分，其余被 GC 回收了；② 全都留住了，但 profile 滞后。我一开始倾向 ①。

实验（2026-10-04 重新压测，202 仍是 1500）：同一时刻分别不加和加 `?gc=1` 抓 heap。

| <br />   | 不加 `gc=1`       | 加 `gc=1`（先 GC 再抓） |
| -------- | --------------- | ----------------- |
| 总量       | 30.2MB（30971kB） | 100.99MB          |
| `ingest` | 23.9MB（24509kB） | **94.68MB**       |

```
Showing nodes accounting for 100.49MB, 99.50% of 100.99MB total
      flat  flat%   sum%        cum   cum%
   94.68MB 93.75% 93.75%    94.68MB 93.75%  main.(*server).ingest
    3.50MB  3.47% 97.22%     3.50MB  3.47%  runtime.mallocgc
    2.30MB  2.28% 99.50%     2.30MB  2.28%  main.newDedup (inline)
```

结论：**是 ②**。强制 GC 后 94.68MB ≈ 93.75MB，1500 块 64KB 一块都没被回收。
不加 `gc=1` 时数字小，是因为 heap profile 的 `inuse` 是**最近一次 GC 结束时**的快照，那之后分配的不算（讲义 §5.4b）。

三个追问：

- **94.68 比 93.75 多了约 1%，为什么？**
  我的答案：采样平均每 512KB 记一次，最后不到 512KB 的部分没统计进去。
  ❌ 方向矛盾：这个解释只会让 profile **少算**，实际却**多了**。
  正解：采样是**随机**的，不是攒满 512KB 记一次。每块 64KB 约有 12% 的概率被记下，被记下的按「代表约 8 块」放大。
  结果是统计估算，没有系统偏差，但有随机误差，**可多可少**。
  ⭐ 方法：提出解释后，先看它预测的方向和数据是否一致，方向对不上就可以排除。
- **是 ① 还是 ②？**
  我的答案：②，一开始差别大只是 profile 滞后。✅
  措辞更准的说法：不是「下一次 GC 还没开始」，而是「**最后一次 GC 之后分配的**没被算进去」。
- **同样 1500 个** **`/ingest`、同样不加** **`gc=1`，上次 37.23MB，这次 23.9MB，为什么？**
  我的答案：这次最后一次 GC 发生得更早。✅
  GC 发生的时刻取决于堆增长速度、调度和其它分配，每次运行都不同，所以同样的负载会抓到不同的数字。
  这个现象本身也支持 ②：如果是 ①，「留住多少」由代码逻辑决定，每次结果应该稳定。

⚠️ 第 1 步那两个 heap 文件都没加 `gc=1`，所以 37.23MB 这个增量也受了滞后的影响。用 `-base` 比较时，两次都要加 `gc=1` 才是在比同一种状态。

1. 对照：
   三次的 goroutine profile结果如下：
   施加压力前：
   goroutine profile: total 7
   施加压力后：
   goroutine profile: total 748
   等了30秒后
   goroutine profile: total 748
   压测程序打印的那几行:
   3000 个请求，用时 2.87s
   status 504: 741
   status 200: 759
   status 202: 1500

压力测试后，goroutine的数量从7涨到了748，并且30秒后也没有回落，说明有泄漏。增量741（748-7）个goroutine的数量正好等于504出错的那几个请求的数量。

1. 根因：

**goroutine 泄漏**

```shell
grep -m1 -A5 '^goroutine .*\[chan' profile/d16/goroutine-after-debug2.txt
goroutine 332 [chan send, 68 minutes]:
main.withTimeout[...].func1()
        /Users/hzjconan/workspace/learn-program-language/go/cmd/leakdemo/helpers.go:20 +0x38
created by main.withTimeout[...] in goroutine 67
        /Users/hzjconan/workspace/learn-program-language/go/cmd/leakdemo/helpers.go:20 +0xd0
```

helpers.go的20行会对每次被调用时候传进来的fn参数（每个reqeust），启用一个goroutine。server.go里实际上把61行定义的fetchPrice函数当作fn传给了helpers.go的withTimeout函数，并且把50毫秒作为参数d传给了withTimeout函数。fetchPrice函数本身不支持超时退出，那么当fetchPrice的执行时间超过50毫秒的时候，会导致helpers.go的25行被执行到，最终结束了这个函数的执行，那也就是说23行这里从ch读数据的这行就执行不到了，并且因为19行在定义这个ch的时候，用的是无缓冲的，所以它会等接受者来读，
因此造成了goroutine泄漏。
“增量741（748-7）个goroutine的数量正好等于504出错的那几个请求的数量”，是因为timeout过后，执行了helpers.go的25行分支并且走到了28行里，
最终输出了http 504的response（Gateway Timeout）。在这个测试里，fetchPrice执行时间超过了50毫秒是唯一导致helpers.go的28行被执行的原因，所以741会正好等于response code是504的请求的个数。如果是因为客户端中途断开链接，会走到helpers.go的第30行，也会造成goroutine泄漏。但这时候泄漏数就不一定等于504的请求个数了。
在这个实验里，不存在客户端主动断开的情况，因为load.go的22行定义了超时时间是5秒，远大于50毫秒的超时时间。

**内存泄漏**

谁拿着 `buf`：顺着 `server.go:71` 分配的 `buf` 往下读。

- 第 72 行 `io.ReadFull(..., buf)` 只是往 `buf` 里写数据，写完就返回，**不会留住它**
- 第 82 行 `s.seen.add(buf[:sigLen])` 把 `buf` 的前 16 字节切片交给了 `dedup`，而 `dedup` 挂在 `server` 上，**和进程活得一样久**

`dedup.add` 里，`set` 和 `order` 存的东西不一样（`helpers.go`）：

```go
59:  if _, ok := d.set[string(sig)]; ok {      // string(sig) 新分配一块内存、复制 16 字节
63:      d.order = append(d.order, sig)          // 存的是切片本身：指向 buf 底层数组的指针
69:  d.set[string(sig)] = struct{}{}             // set 的 key 是那份独立拷贝，和 buf 无关
```

所以拿着 64KB 不放的是 **`order`**（`[][]byte`），不是 `set`。
（我一开始答成了 `set`，❌。`string(sig)` 是拷贝，`order` 存的才是共享底层数组的切片。）

为什么 16 字节的切片能留住 64KB：切片是 `{ptr, len, cap}` 三样东西（D3）。

```
buf        = { ptr → [64KB 底层数组的开头], len 65536, cap 65536 }
buf[:16]   = { ptr → [同一个数组的开头],    len 16,    cap 65536 }
                └── 同一个指针
```

`buf[:16]` 不是拷贝，它和 `buf` 共享同一个底层数组。GC 以**一整块分配**为单位回收：
只要还有任何指针指向这块数组的任何位置，整块 64KB 都不能回收，GC 没法只释放没用到的 65520 字节。

「内存有上限」是错的：`newDedup(50_000)`（`server.go:23`），上限是 50000 条。

- 按簿记开销算（我一开始的算法，❌）：`order` 一个切片头 24 字节 + `set` 一个 16 字节 key ≈ 40 字节，50000 × 40 ≈ **1.9MB**
- 按实际留住的内存算：每条留住一整块 64KB 底层数组，50000 × 64KB = 3,200,000KB ≈ **3.05GB**

⭐ **切片的** **`len`** **只决定你能看到多少；它留住多少内存，要看底层数组。**
上限限制的是**条数**，不是**字节数**；每条的实际大小被一个看不见的切片引用放大了 4096 倍（65536 ÷ 16）。
写这段代码的人和我一开始一样，是按 16 字节算的，所以才敢在注释里写「内存有上限」。
在 512MB 内存上限的 Pod 里，不到 8000 条就会 OOMKill。

1. 修复：
   在helpers.go的64行去拷贝传进来的切片，之后用这个拷贝后的切片，避免占用原始切片的底层数组导致OOM。
   选择在helpers.go里修复而不是调用它的地方， 比如server.go的82行，是因为这样对调用方透明，避免了这次在server.go里修复了，下次别人在别的地方再调用这个add方法导致一样的问题再发生。
2. 回归测试：（`cmd/leakdemo/helpers_test.go`）

**`TestWithTimeout_NoGoroutineLeak`**：表驱动，两个子测试覆盖 `withTimeout` 提前返回的两条路径。

| 子测试   | 怎么让它提前返回                               | 断言                         |
| ----- | -------------------------------------- | -------------------------- |
| 超时    | `fn` 睡 2s，超时 1s                        | `err` 是 `errTimeout`       |
| 调用方取消 | 另起 goroutine 在 500ms 时 `cancel()`，早于超时 | `err` 是 `context.Canceled` |

- 用 `testing/synctest`：bubble 里时间是假的，整个测试 0.00s；bubble 结束时还有 goroutine 永久阻塞就 panic `deadlock`，这就是现成的泄漏检测
- 要点 ①：**断言** **`err`**，确认确实走到了要测的那条路径。我第一版写的是 `err != nil && !errors.Is(...)`，`err == nil` 时也会通过，等于没测
- 要点 ②：`withTimeout` 返回后先 `time.Sleep(fnTime)` 让 `fn` 跑完，**再**断言。
  - 不等的话 `fn` 还在 sleep，synctest 把 sleep 也算阻塞，修复后也会报 deadlock
  - 先断言再 sleep 的话，断言失败时后面会多一个 `sleep (durable)` 的 deadlock panic，看起来像泄漏；而且 panic 让测试进程退出，后面的子测试都不跑（实测）

**`TestDedupAdd_CopySig`**：`newDedup(3)`，从一个 1024 字节的 buffer 里切 `buf[off:off+sigLen]` 传给 `add`。

- 前 3 条走 `append` 分支，第 4 条走「满了覆盖」分支，**两条写入路径都覆盖到**
- 两种断言：
  - `cap(d.order[i]) == sigLen`
  - **污染断言**：`add` 之后把原始 buffer 改成 `0xFF`，存进去的签名不能跟着变

**红绿验证**：用 `go test -overlay` 在编译时替换 `helpers.go`，不改源文件。

| 坏版本                             | 结果                              |
| ------------------------------- | ------------------------------- |
| `make(chan T)` 无缓冲              | 两个子测试都红：`chan send (durable)`   |
| 删掉 `bytes.Clone`                | 红：cap 是 1024/924/824/724，污染断言也红 |
| 只在 `append` 分支复制                | 红：只有覆盖分支红，cap 724               |
| 三下标切片 `sig[:len(sig):len(sig)]` | **cap 断言绿**，污染断言红               |

- ⚠️ synctest 的 deadlock 是 panic，第一个子测试 panic 后整个测试进程就退出了。**验证红要用** **`-run`** **一次只跑一个子测试**，否则以为两个都红了，其实第二个根本没跑
- ⭐ 三下标切片把 `cap` 截成 16，但**仍然共享底层数组**。cap 只是根因的间接症状，可以被伪造；污染断言测的才是根因本身（是否共享底层数组）
- ⚠️ cap 断言还有误报风险：`bytes.Clone` 的容量按分配器规格向上取整，`sigLen` 是 16 时 cap 正好 16；改成 17 或 20 时 cap 是 24，代码正确也会红

1. 验证：（2026-10-04，修复后重新压测，3000 个请求）

```
3000 个请求，用时 2.873s
  status 504: 719
  status 202: 1500
  status 200: 781
```

| <br />                              | 修复前                   | 修复后                     |
| ----------------------------------- | --------------------- | ----------------------- |
| goroutine：压测前 → 压测后 → 等一会儿          | 7 → 748 → 748         | 7 → 7 → 7               |
| `goroutineleak`                     | 741                   | **0**                   |
| 压测期间新增 `inuse`（`-base`，两次都加 `gc=1`） | 94.68MB，全部来自 `ingest` | **513kB**，`ingest` 不在榜上 |

```
Showing nodes accounting for 513kB, 100% of 513kB total
      flat  flat%   sum%        cum   cum%
     513kB   100%   100%      513kB   100%  runtime.mallocgc
         0     0%   100%      513kB   100%  runtime.allocm
         0     0%   100%      513kB   100%  runtime.mcall
         0     0%   100%      513kB   100%  runtime.newm
```

- **504 还是 700 多，这是对的**：修复解决的是泄漏，不是超时。`fetchPrice` 仍有一半左右超过 50ms。
  修复前后 504 都是 700 多，泄漏数从 741 变成 0，说明两者是伴生关系，不是同一件事
- **513kB 不是泄漏**：调用栈是 `runtime.newm → allocm`，runtime 在压测期间多建了一个 OS 线程（M）。
  而且 513kB ≈ 512KB，正好是**一个采样单位**，实际分配可能小得多
- `dedup` 仍然合法地存着 1500 条签名（每条 16 字节 + map 开销，几十 KB），没出现在 `-base` 里，同样是因为远小于采样粒度
- ⚠️ 1500 条压测走不到 `dedup` 的「满了覆盖」分支（容量 50000），所以这次压测**验证不了**那条路径。那条路径靠 `TestDedupAdd_CopySig` 保证
