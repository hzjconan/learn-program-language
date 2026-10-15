# D16 ① operator 指标

> 返回 [D16 笔记](../D16.md) · 讲义 [lessons/D16.md](../../lessons/D16.md) §8

（2026-10-05，本机 `go run ./cmd/main.go --metrics-bind-address=localhost:8090 --metrics-secure=false`；
集群里那份 operator 先缩到 0，做完已恢复成 1）

**一共暴露了哪些指标**：190 个指标名

| 前缀                    | 个数  | 哪一层                                                                  |
| --------------------- | --- | -------------------------------------------------------------------- |
| `go_`                 | 157 | Go 运行时。controller-runtime 注册 Go collector 时用了 `MetricsAll`，全部暴露      |
| `process_`            | 7   | 操作系统进程：CPU、RSS、文件描述符                                                 |
| `controller_runtime_` | 12  | controller-runtime（一个 histogram 占 `_bucket`/`_count`/`_sum` 三个名字）    |
| `workqueue_`          | 11  | client-go 定义接口（`workqueue.MetricsProvider`），controller-runtime 实现并注册 |
| `rest_client_`        | 1   | client-go 定义钩子（`tools/metrics`），controller-runtime 接上并注册             |
| `certwatcher_`        | 2   | controller-runtime 自己的包：监视 metrics/webhook 的 TLS 证书文件，轮换后自动重载        |

- 我一开始把 `certwatcher` 归到 client-go ❌
- ⭐ `workqueue`、`rest_client` 是「**下层定义接口，上层注入实现**」，和 OTel 的 API/SDK 分离、`slog.Handler` 同一个设计。client-go 本身不依赖 Prometheus
- `rest_client_` 默认**只有** `rest_client_requests_total`，耗时等要 `metrics.RegisterRESTClientMetrics(...)` 才注册（讲义 §3.1 原来写错了，已改）
- 启动时什么都没做就有 1 次 reconcile：见 ④ 里「启动时 informer 做了什么」

**四步实验**（讲义 §4.2）

| 操作                   | 我的预测                         | `adds_total` 实际 | 为什么                                                                                                                         |
| -------------------- | ---------------------------- | --------------- | --------------------------------------------------------------------------------------------------------------------------- |
| 空闲 1 分钟              | 不变                           | **+0** ✅        | 没有变化就没有事件；缓存 resync 默认 10 小时（`pkg/cache/cache.go:45`）。也说明 Reconcile 没在自己触发自己（无条件 `Status().Update` 内容没变 → API server no-op） |
| scale Deployment 到 5 | 5                            | **+9**          | 见下面                                                                                                                         |
| annotate WebApp      | 不变，2 次 reconcile（加注解 + 改回去）❌ | **+1**          | 见下面                                                                                                                         |
| annotate Service     | +1，Service `op` 不变，注解留着 ✅    | **+1**，注解还在     | 见下面                                                                                                                         |

*scale 到 5*：开 `kubectl get deploy golearn-api -w` 看 informer 收到的每个事件：

```
GEN   OBSGEN   SPEC   REPLICAS   UPDATED   READY   AVAIL
5     5        2      2          2         2       2      ← watch 启动时的现状，不算事件
6     5        5      2          2         2       2      ← 我：scale 2 → 5
7     5        2      2          2         2       2      ← 我的 operator：改回 2
7     7        2      2          2         2       2      ← Deployment controller：observedGeneration
7     7        2      2          2         2       2      ← Deployment controller：没打印的 status 字段
7     7        2      5          5         2       2      ← 按第 6 代已把 ReplicaSet 扩到 5，3 个 Pod 被创建
7     7        2      2          2         2       2      ← 处理第 7 代，缩回 2
7     7        2      2          2         2       2      ┐
7     7        2      2          2         2       2      ├ Pod 删除过程中 status 其它字段还在变
7     7        2      2          2         2       2      ┘
```

- **我写 1 次、operator 写 1 次、Deployment controller 写 7 次** = 9 个事件 → `adds_total` +9。大部分事件来自 K8s 自己的 controller 写 status
- 这次一个都没去重：reconcile 只要几毫秒，事件间隔更长，每个事件到时 key 已经不在队列里。另一次同样的操作是 +8，**去重多少取决于时机，不是固定数**
- OBSGEN 从 5 跳到 7，但 REPLICAS 确实到过 5：operator 改回去再快，Deployment controller 已经按第 6 代扩容，**那 3 个 Pod 确实被创建又删掉了**
- GEN 每次 spec 变化 +1（我先用一个临时 Deployment 验证过 `kubectl scale` 会让 generation +1）。之前一次 watch 只有一行，是因为 watch 在 scale 结束后才启动

*annotate WebApp*：

- `For(&WebApp{})` 收下 WebApp 的**所有**改动，包括 metadata。generation 不变（只有 spec 变才 +1），但 `resourceVersion` 变了，就有事件
  - 想只在 spec 变时 reconcile：`builder.WithPredicates(predicate.GenerationChangedPredicate{})`，代价是只改 status、只改注解的事件全被丢掉
- operator **不会改回父对象**：WebApp 由用户说了算，operator 只读 spec、只写 status。「改回去」只发生在它管的子对象上
- 我的预测前后矛盾（`adds_total` 不变却有 2 次 reconcile）。预测时先检查各项是否自洽
- 终端 3 没有新行：reconcile 对 Deployment 发了 Update，但内容没变 → API server no-op → 没有事件

*annotate Service*：三项全对。日志里的值是 `"op": "unchanged"`（`OperationResultNone`）。

同一次 reconcile 里的对照：

| <br />     | mutateFn 怎么写的                                             | 每次的 `op`       |
| ---------- | --------------------------------------------------------- | -------------- |
| Service    | 只设 `Type`、`Selector`、`Ports`（`Protocol`、`TargetPort` 都写明） | `unchanged` ✅  |
| Deployment | 整个 `Spec.Template` 覆盖，里面有 API server 会补默认值的字段             | 永远 `updated` ❌ |

注解能留下来，是「只改你管的字段」的另一个好处：Prometheus 抓取配置、云厂商 LB 配置这类别的工具加的注解不会被抹掉，不会来回打架。

**计划外发现 ①：409 Conflict 打成了 ERROR**（✅ 已在练习 ② 3c 修复）

```
ERROR Reconciler error ... "ensure deployment: Operation cannot be fulfilled on deployments.apps \"golearn-api\":
the object has been modified; please apply your changes to the latest version and try again"
```

- 乐观并发：Reconcile 从**缓存**读到旧的 `resourceVersion`，这时 Deployment controller 刚写过 status，带旧版本去 Update → 409 → 退避重试 → 成功
- 三方同时写同一个对象，冲突是必然的，**是正常现象**
- 但日志不合格：ERROR + 堆栈，看告警的人会以为出大事。而且它混进了 `result="error"`，让错误率不可信
- 修法（练习 ② 一起做）：`apierrors.IsConflict(err)` 时 `return ctrl.Result{RequeueAfter: 一个短延迟}, nil`
  - ⚠️ 更正：Claude 最初建议的是 `ctrl.Result{Requeue: true}`，但 controller-runtime v0.25 里 `Requeue` 已标 `Deprecated: Use RequeueAfter instead`（`pkg/reconcile/reconcile.go:41`）
  - `apierrors.IsConflict` 能穿过 `%w` 包装识别 409（`ReasonForError` 内部用 `errors.As`），不用拆错误

**计划外发现 ②：Deployment 每次都是** **`op: updated`**（✅ 已在练习 ② 3b 修复）

- 启动那次、只是 Deployment controller 写 status 的那几次，`op` 全是 `updated`，但 GEN 只变过一次 → API server 实际只承认了 1 次
- 原因：mutateFn 整个覆盖 `Spec.Template`，集群里那份已被补上默认值，每次比较都「不一样」→ 发 Update → API server 补默认值后一样 → no-op（D20 小题 A2）
- D20 的 `TestReconcile_IsIdempotent` 只看 `resourceVersion`，抓不到（测试注释里写过）
- 练习 ② 的 `child_ops_total{op="updated"}` 会把它变成一个持续上涨的数字，到时候一起修

**四个黄金信号**（两次 scale 制造流量后取的累计值）

```
controller_runtime_active_workers{controller="webapp"} 0
controller_runtime_max_concurrent_reconciles{controller="webapp"} 1
controller_runtime_reconcile_time_seconds_sum{controller="webapp"} 0.13692466400000003
controller_runtime_reconcile_time_seconds_count{controller="webapp"} 19
controller_runtime_reconcile_total{controller="webapp",result="error"} 1
controller_runtime_reconcile_total{controller="webapp",result="requeue"} 0
controller_runtime_reconcile_total{controller="webapp",result="requeue_after"} 0
controller_runtime_reconcile_total{controller="webapp",result="success"} 18
workqueue_depth{controller="webapp",name="webapp",priority="-100"} 0
workqueue_depth{controller="webapp",name="webapp",priority="0"} 0
workqueue_queue_duration_seconds_sum{controller="webapp",name="webapp"} 0.11593525099999999
workqueue_queue_duration_seconds_count{controller="webapp",name="webapp"} 19
workqueue_retries_total{controller="webapp",name="webapp"} 1
```

| 信号  | 指标                                                                   | 当前值                                |
| --- | -------------------------------------------------------------------- | ---------------------------------- |
| 延迟  | `reconcile_time_seconds_sum / _count`                                | 平均 7.2ms                           |
| 流量  | `reconcile_total` 各个 result 相加                                       | 19 次                               |
| 错误  | `error / 总次数`                                                        | 1/19 = 5.3%                        |
| 饱和度 | `queue_duration_seconds_sum / _count`；`depth`；`active_workers / max` | 平均排队 6.1ms；积压 0（两档都是 0）；worker 0/1 |

- **健康吗？** 我的答案：基本健康，error 只看这里应该报警，但不知道背后是 409。
  后半句是关键：**这个指标分不清「预期内会自愈的错误」和「真出问题」**。正确做法不是调告警规则，而是**让指标变干净**：冲突走 `RequeueAfter`（计入 `result="requeue_after"`），`error` 里只剩真错误，才能直接拿来告警。
  另外两点：告警看**时间窗口内的速率**（19 次错 1 次是 5.3%，190 次错 1 次是 0.5%）；`retries` +1 之后 `success` 还在涨 = 已经自愈
- **这张表看不出什么？** 我答不知道。答案：**每次 reconcile 都在白写**。四个信号全绿（快、几乎不错、不排队），但正确性看不出来。
  ⭐ 黄金信号衡量「对请求响应得好不好」，衡量不了「做的事对不对、有没有必要」。operator 的正确性要靠自定义指标（练习 ②，讲义 §3.2）

**从 API 请求层面看白写**：预测 PUT 约 38 次

```
rest_client_requests_total{code="200",host="127.0.0.1:55455",method="GET"} 18
rest_client_requests_total{code="200",host="127.0.0.1:55455",method="PUT"} 36
rest_client_requests_total{code="409",host="127.0.0.1:55455",method="PUT"} 1
```

- PUT 200 = 36：18 次成功的 reconcile × 2（Deployment Update + `Status().Update`）
- PUT 409 = 1：失败那次第一个 PUT 就冲突，**直接 return**，后面没执行到。所以是 37 不是 38（我默认了每次都完整跑完）
- 409 这一行和 `result="error"`、`retries_total` 是同一件事在三个层面的样子
- ⭐ 36 次成功写入里**真正改了东西的约 2 次**（两次把 replicas 改回 2），其余约 34 次都是 no-op。500 个 WebApp 的集群里就是每秒几十个白写，API server 限流（429）时会和必要的写入抢配额
- GET 只有 18：如果 `r.Get` 每次都打到 API server，19 次 reconcile 至少要 38 次 GET。实际这 18 次主要是 informer 的 LIST/WATCH 和资源发现。
  ⭐ **读走缓存，写才打到 API server**（D20 §3.3），也是 409 的根源：读到的是稍旧的缓存版本
