# D16 ③ ready_replicas gauge 和僵尸序列

> 返回 [D16 笔记](../D16.md) · 讲义 [lessons/D16.md](../../lessons/D16.md) §8

**这个练习练什么**

一句话：**② 练「把事件变成数字」，③ 练「把状态变成数字」，并且让这个数字随着它描述的对象一起出生、一起消失。**

*① 另一种指标：gauge 报告「现在是什么状态」*

| <br /> | counter（②）      | gauge（③）                 |
| ------ | --------------- | ------------------------ |
| 回答的问题  | 做了多少次某件事        | 某个东西现在是多少                |
| 例子     | 写了多少次 status    | golearn-api 现在有几个 Pod 就绪 |
| 怎么用    | `rate(...)` 看速率 | 直接看值，或和期望值比较             |

用途：按应用画看板、写告警（「某个 WebApp 就绪副本数低于期望、持续 10 分钟」）。
K8s 生态里 **kube-state-metrics** 做的就是这件事：把每个 Deployment、Pod 的状态变成 gauge。③ 是给自己的 CRD 做一个小版本。

*② 用对象名做 label：练习判断基数*

第一次用**对象的名字**做 label，每个 WebApp 一条序列。这次**允许**：上界是集群里 WebApp 的数量，和流量无关。
同样是「每个东西一条序列」，用请求路径、订单号做 label 就不行（讲义 §2.4）。动手前先过一遍这个判断。

*③ ⭐ 重点：序列的生命周期要跟着对象走*

gauge 的序列存在 **operator 进程内存**里：第一次 `WithLabelValues("default", "golearn-api")` 时创建，之后**除非主动删除，一直存在**，直到进程重启。

WebApp 被删了而什么都不做：

- `golearn_webapp_ready_replicas{name="golearn-api"}` **一直停在 2**，看板显示一个不存在的应用「2 个副本就绪」
- 如果停在 0，「就绪副本数低于期望」的告警**永远响下去**，针对一个早就删掉的应用

这就是**僵尸序列**（讲义 §2.5）。对象出现时创建序列，对象消失时删掉序列。reconcile 里「对象消失」= **NotFound 路径**。

⭐ 这修正了 D20 的一条规则。D20 说 NotFound 时**什么都不用做**，子资源由 K8s GC 删。仍然成立，但它只覆盖 **K8s 里的东西**。
operator 在**自己进程里**为这个对象维护的状态（指标就是一种），K8s 不知道，不会替你清，只能自己在 NotFound 时清掉。
和 D20 §4.5 finalizer 同一个道理：**K8s 之外的东西，K8s 不负责清理。** 区别是指标在自己进程里，reconcile 一定能看到 NotFound，不需要 finalizer。

*顺带练：怎么测「某个东西不存在了」*

测「值是 2」容易，测「序列被删了」要动脑筋：直接去读（`WithLabelValues`）反而会把它**重新建出来**。**观察行为本身改变了被观察的东西。**

**动手前的两个问题**

- **基数**：我的答案：WebApp 在一个 namespace 下唯一；用 `r.URL.Path` 会让不同路径的流量分别统计。❌ 都是事实，但都不是判据
  - 「唯一」不保证安全：订单号也唯一，照样出事故
  - 「分别统计」本来就是 label 的作用，按 `method` 分开完全没问题
  - ⭐ **判据：label 的取值个数必须有上界，而且这个上界和流量无关**

| label                         | 不同的值有多少  | 由什么决定                           |
| ----------------------------- | -------- | ------------------------------- |
| WebApp 的 `namespace` + `name` | 几个到几百个   | **人**创建了多少 WebApp               |
| `r.URL.Path`                  | **没有上界** | **请求**：`/orders/1`、`/orders/2`… |
| `r.Pattern`                   | 几十个      | 代码里注册了多少路由                      |

- `r.URL.Path` 更危险的是值**由请求方决定**：任何人发一堆 `/xxx1`、`/xxx2` 就能给服务和 Prometheus 加序列，成了不需要认证的内存耗尽攻击
- **僵尸序列**：我的答案：序列还在，删 WebApp 时也该删掉 ✅。它会一直停在最后一次 `Set` 的值（比如 2）

**实现**（最终：10 个测试全绿，`make lint` 0 issues）

```go
// metrics.go
readyReplicas = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "golearn_webapp_ready_replicas",
		Help: "Ready replicas of the Deployment managed by a WebApp, as observed at the last reconcile.",
	},
	[]string{"namespace", "name"},
)
// init(): metrics.Registry.MustRegister(readyReplicas)

// reconcile 开头
if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
	if apierrors.IsNotFound(err) {
		// WebApp 已删除：子资源由 GC 处理，但 operator 进程里的指标要自己清（否则成了僵尸序列）
		readyReplicas.DeleteLabelValues(req.Namespace, req.Name)
	}
	return client.IgnoreNotFound(err)
}

// reconcile 末尾，在 if statusChanged 块【外面】
readyReplicas.WithLabelValues(app.Namespace, app.Name).Set(float64(ready))
```

- namespace、name 从 `req` 拿：NotFound 时 `app` 里什么都没有。`ctrl.Request` 内嵌 `NamespacedName`，直接 `req.Namespace`（写 `req.NamespacedName.Namespace` lint 会报 QF1008）
- **`Set`** **放在** **`if statusChanged`** **外面**：
  - `statusChanged` 回答「要不要写 API server」，写入有成本，所以先比较
  - gauge 回答「此刻是多少」，`Set` 只是写内存，每次 reconcile 都该报告
  - 放在里面的话：operator 重启后内存里的 gauge 是空的，启动那次 status 没变 → 跳过 `Set` → 直到下次 status 变化前，这个应用在 `/metrics` 上不存在
  - ⚠️ 现有测试抓不到这个（测试里每次 reconcile 时 status 都恰好变了）。想补：`DeleteLabelValues` 模拟重启 → 什么都不改再 reconcile → 断言序列又出现

**测试** **`TestMetrics_ReadyReplicasGauge`**

```
① 建 WebApp → reconcile
② 用 k8sClient.Status().Update 把 Deployment 的 ReadyReplicas 改成 2 → reconcile   （envtest 没有 Deployment controller，不改永远是 0）
③ 断言 gauge == 2；再从 metrics.Registry GatherAndCount，确认注册了
④ k8sClient.Delete(app) → reconcile   （没有 finalizer，Delete 后立刻消失 → reconcile 走 NotFound）
⑤ readyReplicas.DeleteLabelValues(ns, name) 返回 true → 序列还在 → 僵尸序列
```

- **怎么判断序列不存在**：不要用 `ToFloat64(WithLabelValues(...))`，`WithLabelValues` 发现不存在会**新建一条**返回 0，分不清，还把僵尸造回来了。
  用 `DeleteLabelValues` 的返回值：删除前存在返回 `true`
- **测结果，不测实现**：问的是「怎么验证 `DeleteLabelValues` 被调用了」→ 应该验证「序列不在了」。
  换成 `DeletePartialMatch` 批量删时，「验证调用了某函数」会误报；传错参数时，「被调用了」照样成立但序列没删
- **第 ③ 步不能省**：如果 ② 根本没设置 gauge，序列从没存在过，⑤ 也会过。先确认前提成立，再断言结果（和 `updates > 0`、`errTimeout` 同一条规则）
- 两条断言测的不是一回事：

| <br />                                                   | 读的是什么                      | 回答                |
| -------------------------------------------------------- | -------------------------- | ----------------- |
| `testutil.ToFloat64(readyReplicas.WithLabelValues(...))` | Go 变量本身                    | reconcile 有没有把值设对 |
| `testutil.GatherAndCount(metrics.Registry, "...")`       | registry = `/metrics` 实际输出 | Prometheus 看不看得到  |

**这一节测试抓到的两个真 bug**

1. ❌ **gauge 没注册**：`init()` 里只注册了两个 counter。直接读变量的断言照样过（小题 A3 的更隐蔽版：A3 是注册错地方，这里是完全没注册）。
   加了 `GatherAndCount` 才抓到。没注册时 `err` 是 nil、`n` 是 0，错误消息要把两种情况分开，否则打出来是 `GatherAndCount: <nil>`，什么都看不出来
2. ❌ **删除条件写反**：第一次拆 NotFound 时写成

```go
e := client.IgnoreNotFound(err)
if e != nil { readyReplicas.DeleteLabelValues(...) }   // e != nil = 【不是】NotFound
```

| 失败原因                  | 会不会删   | 应不应该删 |
| --------------------- | ------ | ----- |
| NotFound（WebApp 被删了）  | **不删** | 该删 ❌  |
| 超时、503、403（WebApp 还在） | **删**  | 不该删 ❌ |

两种情况正好反了：真删了的留下僵尸，暂时读不到的反而被清掉（看板时有时无，`absent()` 告警误触发）。
和 3a 里 `observedGenerationChanged` 用 `==` 同一类：**用间接的条件表达意图，把方向弄反了**。`IgnoreNotFound` 是为「返回什么」设计的，拿它判断「是不是 NotFound」绕了一道弯；直接用 `apierrors.IsNotFound`。
⭐ 第 ⑤ 步断言在**我以为已经改好的时候**亮了红灯。没有它，这段代码会直接上线

- 红绿验证：overlay 去掉 `DeleteLabelValues` 那一行 → 第 ⑤ 步红（僵尸序列）；恢复 → 绿
- ⭐ 总结：**测试要验证外部真正能看到的结果**（Prometheus 抓到了什么、序列还在不在），而不是代码内部某个变量的值
