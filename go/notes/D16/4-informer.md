# D16 ④ 数 informer

> 返回 [D16 笔记](../D16.md) · 讲义 [lessons/D16.md](../../lessons/D16.md) §8

（2026-10-05，本机 `go run ./cmd/main.go --pprof-bind-address=localhost:6061` 连 kind 集群，等 `Starting workers` 后抓 `goroutine?debug=1`，存在 `profile/d16/operator-goroutine.txt`）

- goroutine 总数：**58**
  （总数随环境变：Claude 备课时在 envtest 里数到 54，连真集群多几个。重要的是下面几组的数量。）
- `Reflector.watch` 有几个、分别对应哪几个 informer：**3 个**

```
3 @ 0x1025a7ddc 0x102585f3c ...
#    k8s.io/client-go/tools/cache.handleAnyWatch+0x1e3        .../reflector.go:1001
#    k8s.io/client-go/tools/cache.handleWatch+0x6b            .../reflector.go:958
#    k8s.io/client-go/tools/cache.(*Reflector).watch+0x4bf    .../reflector.go:620
```

对应 `webapp_controller.go` `SetupWithManager` 里的三行：

```go
265: For(&appsv1alpha1.WebApp{}).      → WebApp 的 informer
266: Owns(&appsv1.Deployment{}).       → Deployment 的 informer
267: Owns(&corev1.Service{}).          → Service 的 informer
```

- `processorListener).run` **3 个**，`processorListener).pop` **3 个**

```
3 @ ...
#    k8s.io/client-go/tools/cache.(*processorListener).run+0x73    .../shared_informer.go:1487
3 @ ...
#    k8s.io/client-go/tools/cache.(*processorListener).pop+0x10b   .../shared_informer.go:1452
```

**⭐ 到这里才对 informer 有了直观的理解**（D19 笔记写过「informer 还没搞懂」，D20 §3 重讲过，这次是从进程内部看到的）：

> **每个 informer 在进程里就是一个长期运行的** **`Reflector.watch`** **goroutine。**
> **它和 API server 保持着一条 watch 长连接，有变化就推过来。**
> profile 里那 3 个 `Reflector.watch`，就是 3 条长连接。

一个 informer 里的分工：

```
API server ──watch 长连接──► Reflector.watch       （每个 informer 1 个）
                                 │ 收到变化，写进缓存
                                 ▼
                        processorListener.pop      （每个事件处理函数 1 个）
                                 │ 取出事件
                                 ▼
                        processorListener.run      （每个事件处理函数 1 个）
                                 │ 调用处理函数 → 把 WebApp 的 key 放进 workqueue
                                 ▼
                           processNextWorkItem → Reconcile
```

- `pop`、`run` 各 3 个 = 每个 informer 上挂了**一个**事件处理函数，就是我这个 controller 的
- 如果同一进程里还有第二个 controller 也 watch Deployment：Deployment 的 informer **还是一个**（缓存共享），但它上面会多挂一组 `pop` + `run`
- `For` / `Owns` 这几行代码，在进程里的样子就是「多一组这样的 goroutine + 一条到 API server 的长连接」

**启动时 informer 做了什么**（练习 ① 开 metrics 时看到的，什么都没操作就已经有一次 reconcile）

```
workqueue_adds_total{controller="webapp",name="webapp"} 1
controller_runtime_reconcile_total{controller="webapp",result="success"} 1
workqueue_depth{controller="webapp",name="webapp",priority="-100"} 0
```

我的答案：informer 收到全局的变化；是 1 因为集群里只有 1 个 WebApp。方向对，两处不准确：

- **不是「变化」，是启动时的全量 LIST**：informer 先把现有对象全部 LIST 一遍，每个对象发一个 Add 事件（带 `IsInInitialList: true`），之后才开始 watch 真正的变化。
  所以 operator 重启后会把所有已有对象 reconcile 一遍：它不知道停机期间发生过什么，从头检查（D19「只看现状，不信事件」）
- **「正好是 1」= 1 个 key + 队列去重**：三个 informer 启动时都做了 LIST

```
WebApp informer      LIST → golearn-api（WebApp）     → key: golearn-api
Deployment informer  LIST → golearn-api（Deployment） → 找 owner → key: golearn-api
Service informer     LIST → golearn-api（Service）    → 找 owner → key: golearn-api
```

3 个事件指向同一个 key。key 在等待处理期间，再来多少事件都只算一次入队：

```go
// controller-runtime pkg/controller/priorityqueue/priorityqueue.go:235-249
if _, ok := w.items[key]; !ok {      // 队列里还没有这个 key
    w.items[key] = item
    w.metrics.add(key, item.Priority) // ← 只有这时才算一次 add
}
```

⭐ **3 个事件 → 1 次入队 → 1 次 reconcile**。「1 个 WebApp」决定只有 1 个 key；「队列去重」决定 3 个事件只算 1 次。

- **`priority="-100"`**：启动时全量 LIST 产生的事件是低优先级，真正的新变化默认是 0

```go
// pkg/handler/eventhandler.go:136-137, 178
if e.IsInInitialList {
    wq.priority = new(LowPriority)
}
const LowPriority = -100
```

operator 重启时补发的大量旧对象排在用户刚做的修改后面，不会把它堵住。`workqueue_depth` 按优先级分档统计，`-100` 那档是 0 = 启动补发的已经处理完了

- 加 `Owns(&corev1.ConfigMap{})` 的预测 / 实测：（存在 `profile/d16/operator-goroutine-cm.txt`，验证完已删掉这行）

| <br />            | 预测          | 实测          |
| ----------------- | ----------- | ----------- |
| `Reflector.watch` | 4           | **4** ✅     |
| goroutine 总数      | 58 + 7 = 65 | **69**（+11） |

为什么是 +11：按「每组几个 goroutine」统计两个文件。

| 每组数量 | 加之前      | 加之后                 |
| ---- | -------- | ------------------- |
| 1    | 16 组     | 16 组（和 informer 无关） |
| 2    | 3 组      | 2 组                 |
| 3    | **10 组** | 1 组                 |
| 4    | —        | **10 组**            |
| 6    | 1 组      | 1 组（和 informer 无关）  |

- 「3 个一组」实际是 **10** 组，不是我估的 7 组，全部变成「4 个一组」→ +10
- 还有 1 个藏在「2 个一组」里：接收 watch 数据流的 `StreamWatcher.receive`，每个 informer 一个，但被**拆成了两组**：

```
StreamWatcher.receive ×2 → lengthDelimitedFrameReader      ← Deployment、Service：protobuf
StreamWatcher.receive ×1 → json.Decoder / jsonFrameReader  ← WebApp（CRD）：只能用 JSON
```

加了 ConfigMap（内置类型，protobuf）后 protobuf 那组 2 → 3 → +1

- **10 + 1 = 11 = 69 − 58**。每个 informer 大约带 **11 个 goroutine**

⚠️ 读 goroutine profile 的陷阱：同一类 goroutine 可能因为调用栈稍有不同（这里是解码格式）被拆成几组，按「每组数量」推算时要留意。

**这个 ConfigMap informer 在 watch 什么？**

我一开始的答案：WebApp 没建任何 ConfigMap，所以它收不到消息。❌

实际：它收下**全集群所有 ConfigMap**（每个 namespace 的 `kube-root-ca.crt`、kube-system 里的组件配置……）并全部缓存在 operator 进程里。
「顺着 ownerReference 找父 WebApp」是在**收到之后**、事件处理函数里做的，API server 不替你过滤。
准确的说法是：**Reconcile 收不到触发**，但 informer 收到了所有消息，缓存里也存了所有对象。

⭐ 每加一个 `For` / `Owns` / `Watches`，代价是「缓存整个集群里这种类型的对象」。详见 QA #25。

- `processNextWorkItem` 有几个、由什么决定：**1 个**，由 `MaxConcurrentReconciles` 决定，默认 1 ✅

```
1 @ ...
#    sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue.(*priorityqueue[...]).GetWithPriority+0x157   .../priorityqueue.go:461
#    sigs.k8s.io/controller-runtime/pkg/internal/controller.(*Controller[...]).processNextWorkItem+0x47        .../controller.go:423
```

- 栈顶停在 `GetWithPriority`：worker 在**等队列里来 key**。数量固定、本来就该一直等，是正常的阻塞，不是泄漏
- 用的是 `priorityqueue`：较新的 controller-runtime 换成了优先级队列，重启时为已有对象补发的大量事件排在真正的新变化后面
- 调大：`SetupWithManager` 里加 `WithOptions(controller.Options{MaxConcurrentReconciles: 4})`，这一组就会变成 4。调大前要确认的事见 D20 小题 B5
- 和指标的关系：`controller_runtime_max_concurrent_reconciles` 是配置值，`controller_runtime_active_workers` 是此刻在干活的数。两者相等且队列在积压 → worker 不够用（讲义 §3.1 饱和度）
