# D17 ③ GC 实验

> 返回 [D17 笔记](../D17.md) · 讲义 [lessons/D17.md](../../lessons/D17.md) §3

（2026-10-16 → 10-17。输入：`cmd/loggen -n 1000000`，约 61MB）

**实验步骤**

```bash
go run ./cmd/loggen -n 1000000 > profile/d17/big.log
mkdir -p profile/d17/bin                          # 可执行文件单独放一个子目录
git stash                                          # 收起练习 ② 的改动 → 编译优化前的版本
go build -o profile/d17/bin/loganalyze-old ./cmd/loganalyze
git stash pop                                      # 恢复，git status 确认改动都在
go build -o profile/d17/bin/loganalyze-new ./cmd/loganalyze

# 跑一次：报告丢掉，gctrace 和 time 的输出（都在 stderr）写进文件
GODEBUG=gctrace=1 ./profile/d17/bin/loganalyze-old profile/d17/big.log > /dev/null 2> profile/d17/gctrace-old.txt
```

多种配置的对比放在 `bash -c '...'` 里循环跑（见最后的 `run` 函数），结果文件是 `profile/d17/gc-*.txt`。

`profile/d17/` 下：`bin/` 是编译出来的两个程序；`big.log` 是输入；`gc-*.txt`、`gctrace-old.txt` 是 gctrace 输出；
`base*.txt`、`step*.txt` 是练习 ② 的 benchstat 数据；`*-cpu.out`、`*-mem.out` 是练习 ② 的 pprof profile。

## 1. 读 gctrace（旧版本，默认配置）

```
gc 19 @0.229s 2%: 0.038+3.0+0.008 ms clock, ..., 216->218->136 MB, 222 MB goal, 0 MB stacks, 0 MB globals, 10 P
```

- 存活堆：**136MB**（`->136 MB`）✅
- GC 占 CPU：**2%**。⚠️ 这是**到目前为止的累计值**，不是每次 2%。最后一次 GC 在 0.229s，程序要跑约 0.6s，之后只排序、出报告，所以整个程序算下来更低。**GC 不是这个程序的瓶颈**
- 存活堆 92 → 111 → 136MB 一直涨，**不是泄漏**：`loganalyze` 设计上就是把所有记录存到最后。
  长期运行的服务流量稳定时存活堆还一直涨才是泄漏；批处理按设计累积数据，涨是正常的

## 2. 不同 GOGC（旧版本）

| GOGC | GC 次数 | 最后一次 | 耗时 |
|---|---|---|---|
| 50 | 25 | 310->310->132 MB | **0.70s** |
| 100 | 20 | 214->215->133 MB | 0.60s |
| 400 | 5 | 327->327->213 MB | 0.61s |
| off | 0 | — | 0.60s |

- GOGC 越大，能容忍的堆增长越多，GC 越少 ✅
- 耗时基本没差别 ✅：GC 是并发的，10 核里有空闲的核替它干活。在 K8s 容器里只给 1～2 核时，GC 和业务抢 CPU，差距会大得多（讲义 §4.5）
- 唯一能看出差别的是 **GOGC=50 慢了约 17%**：GC 频繁，标记期间分配又快，触发 **mark assist**，分配的 goroutine 被拉去帮 GC 干活（讲义 §3.6）
- ⚠️ **「最后一次」不能横着比**：每种配置的最后一次 GC 发生在不同时刻，而存活堆一直在涨。GOGC=400 的 213MB 是因为最后一次 GC 发生得晚、已经读了更多数据，不是它让数据变多了

## 3. GOMEMLIMIT 设得比存活数据还小 & 4. 新旧版本对比

| | GC 次数 | 最后一次 | 耗时 | 最大 RSS |
|---|---|---|---|---|
| 旧，默认 | 20 | 216->217->135 MB | 0.67s | 347MB |
| 旧，`GOMEMLIMIT=100MiB` | **104** | 208->208->132 MB | 0.89s | 275MB |
| 新，默认 | **11** | 278->278->191 MB | 0.27s（见下） | 302MB |

**第 3 问** ✅：GC 次数涨了 5 倍，耗时多了 33%，RSS 下降了一些，但**没压到 100MiB 以下**。存活数据本身超过 130MB，`GOMEMLIMIT` 是软上限，快到上限时 GC 拼命跑，压不下去就允许超过。
补：次数涨 5 倍、耗时只多 33%，因为大部分 GC 工作由空闲核承担；runtime 也会把 GC 限制在约 50% CPU，防止死亡螺旋（讲义 §3.4）

**第 4 问**：

- GC 次数 20 → 11：练习 ② 去掉了每行**两种**分配（多余的 `Text()`、`SplitN` 的 `[]string`），每行 3 次 → 1 次。
  更准确地说，决定 GC 次数的是分配的**字节数**（136 → 91MiB），不是次数：GC 按「堆长了多少」触发
- 存活堆 135 → 191MB：我答成了「最后一次 GC 发生得更早」❌，**反了**。
  「早晚」不能看 `@0.191s` 这种钟表时间（新版本整体快）；要看那时**已经读了多少数据**，这个程序里存活堆的大小基本就代表读了多少行。
  新版本分配少，两次 GC 之间要读更多行，间隔被拉长，最后一次落在了读取过程中**更靠后**的位置 → 存的记录更多 → 存活堆更大。
  和 GOGC=400 同一个陷阱：**GC 越少，最后一次越晚**。真正比较最终存活量，要在同一时刻测（读完后强制 GC，D16 `?gc=1`）

## 踩到的坑

- **zsh 的 `MULTIOS`**：`... 2>&1 >/dev/null | tail` 在 bash 里是「stderr 进管道、stdout 丢掉」，在 zsh 里 stdout 会**同时**进 `/dev/null` 和管道，`tail` 看到的是报告而不是 gctrace。改成 `> /dev/null 2> 文件` 两个重定向各管各的；循环放进 `bash -c '...'`
- **单次耗时不可信**：第一次跑新版本是 **3.36s**（比旧版慢 5 倍），连跑三轮都是 0.26～0.28s。最可能是 macOS 第一次运行新编译的程序时做安全扫描。
  一个和所有其它数据都矛盾的结果，第一反应是「再跑几次」；看 `time` 的 `user`：`real` 大、`user` 小说明时间花在等待上
- 直接运行，新版本在 20 万 / 50 万 / 100 万行时都更快（0.06 / 0.13 / 0.29s vs 0.12 / 0.30 / 0.66s），和 benchmark 的结论一致

## `run` 函数最终执行的命令

`run old-limit100 ./profile/d17/bin/loganalyze-old GOGC=100 GOMEMLIMIT=100MiB` →
`name=$1; bin=$2; shift 2` 后 `"$@"` = `GOGC=100 GOMEMLIMIT=100MiB` →

```bash
env GOGC=100 GOMEMLIMIT=100MiB GODEBUG=gctrace=1 \
    /usr/bin/time -l ./profile/d17/bin/loganalyze-old profile/d17/big.log \
    > /dev/null 2> profile/d17/gc-old-limit100.txt
```

`env A=1 程序`：只对这一个程序设环境变量。`"$@"` 让每个参数保持独立（`$*` 会按空格拆开）。后面几行只是从文件里把数字 `grep` 出来。
