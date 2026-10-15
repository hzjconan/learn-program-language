# D16 ② 自定义指标 + 修 Reconcile

> 返回 [D16 笔记](../D16.md) · 讲义 [lessons/D16.md](../../lessons/D16.md) §8

（2026-10-05 → 10-08。最终：9 个测试全绿，`make lint` 0 issues）

| <br />     | 修之前                            | 修之后                                                                                         |
| ---------- | ------------------------------ | ------------------------------------------------------------------------------------------- |
| 指标         | 只有 controller-runtime 自带的      | 新增 `golearn_webapp_child_ops_total{kind,op}`、`golearn_webapp_status_updates_total`          |
| Deployment | 每次都 `updated`（白写）              | 没变化时 `unchanged`；探针只设 Path/Port，保留服务端默认值                                                    |
| status     | 无条件写（D20 小题 B3）                | 先比较，有变化才写；计数、日志紧贴着写入                                                                        |
| 409 冲突     | ERROR + 堆栈，计入 `result="error"` | `RequeueAfter: 3s`，不算错误                                                                     |
| 测试         | 8 个                            | 9 个：+ `TestMetrics_IdempotentReconcileIsVisible`、`TestReconcile_ConflictIsRequeuedNotError` |

**第 1 步：定义 counter，接进 Reconcile**（`metrics.go`）

- 注册到 `metrics.Registry`（controller-runtime 的），不是 `prometheus.MustRegister`；在 `init()` 里注册（重复注册会 panic）
- `go mod tidy` 后 `client_golang` 从 indirect 变成直接依赖（`kylelemons/godebug` 是 `testutil` 带进来的）
- `op` 是 `controllerutil.OperationResult`，底层是 `string`，用 `string(op)`。它**没有** `String()` 方法
- **Inc 放在 err 检查之后**。我的答案：出错了不该 +1。更具体的理由：
  `CreateOrUpdate` 失败时返回 `OperationResultNone` = `"unchanged"`。放在前面的话，那次 409 会被记成 `op="unchanged"`，意思变成「检查过了，不用改」，
  实际是「要改但写失败了」。**`unchanged`** **代表幂等成立，失败也算进去就会在最需要说实话的时候说错话。** 失败已经有 `reconcile_total{result="error"}`、`rest_client_requests_total{code="409"}` 统计
- `status_updates_total` 的 Inc 也在 err 之后 → 数的是**成功**的调用。但「成功」包括 API server 当作 no-op 的写入（白写同样返回 200）
- Help 文字写明这两点：

```go
Help: "Successful CreateOrUpdate calls on WebApp child resources, by kind and result. " +
	"op=\"updated\" means the operator sent an Update; the API server may still treat it as a no-op.",
Help: "Successful Status().Update calls on WebApp, including ones the API server treats as no-op.",
```

**第 2 步：测试先红**（`TestMetrics_IdempotentReconcileIsVisible`）

同一个 WebApp reconcile 两次，断言第二次：`child_ops_total{kind="Deployment",op="unchanged"}` +1；`status_updates_total` 不变。counter 是包级共享、只增不减 → **断言差值**。

- ❌ 我第一版把第一条断言换成了 **Service**（注释里写着「Deployment 每次都 updated」）。Service 本来就是 `unchanged`，于是绿了。
  但这条断言要抓的**就是** Deployment 每次 `updated`。换成 Service 后：现在绿、修好后绿、以后有人改坏还是绿 → 什么都证明不了
- ⭐ **测试红了是对的，这正是它的价值。** 换回 Deployment 后两条都红：`Deployment unchanged 差 = 0`、`status_updates_total 差 = 1`

**3a. status：先比较，有变化才写**

- ❌ 我第一版：

```go
observedGenerationChanged := app.Status.ObservedGeneration == app.Generation   // 名字叫 Changed，用的却是 ==
readyReplicasChanged := app.Status.ReadyReplicas == deploy.Status.ReadyReplicas
...
if observedGenerationChanged || readyReplicasChanged { Status().Update }       // 写入由这个条件决定
if changed { statusUpdatesTotal.Inc() }                                         // 计数却由 condition 决定
```

| ObservedGeneration | ReadyReplicas | 会不会写   | 应不应该写          |
| ------------------ | ------------- | ------ | -------------- |
| 没变                 | 没变            | **写**  | 不该 ❌           |
| 变了                 | 没变            | 写      | 该 ✅            |
| 没变                 | 变了            | 写      | 该 ✅            |
| 变了                 | 变了            | **不写** | 该 ❌（status 丢失） |

测试却绿了：第二次 condition 没变 → 没 Inc → 差 0。但 `Status().Update` **照样被调用了**。
和 dedup 里「三下标切片骗过 cap 断言」同一类：测的是间接症状，症状消失测试就过，根因还在。

- ⭐ **计数器要紧贴在它统计的那个动作旁边**：Inc 放在 `Status().Update` 成功之后、同一个 `if` 块里。日志 `status updated` 也一样，原来无条件打印，也在说谎
- 改对后：赋值**之前**用 `!=` 比较 → `SetStatusCondition` 的返回值也并进同一个 `statusChanged` → `if statusChanged { Update; Inc; log }`
- ⚠️ 已知缺口：「generation 和 readyReplicas **同时**变化」这个场景没有测试覆盖（`TestReconcile_SetsStatus` 每次只改一项），靠代码逻辑保证。以后改这段要小心

**3b. Deployment：只改自己管的字段**（D20 §4.2 的 B 档）

- 先改成 B 档、但探针整个覆盖 → **还是红**：API server 给探针补了 `Scheme: HTTP`、`TimeoutSeconds: 1`、`PeriodSeconds: 10`、`SuccessThreshold: 1`、`FailureThreshold: 3`，每次比较都不一样。是 A 档问题的缩小版
- 改成 mutateFn 里只设 `HTTPGet.Path` / `HTTPGet.Port`：先判 nil（第一次创建时探针不存在），**在原对象上改**，服务端补的字段保留下来 → 全绿
- ⚠️ `p.Exec, p.TCPSocket, p.GRPC = nil, nil, nil`：探针的四种处理方式只能选一种，有人改成别的，不清掉的话 Update 会被拒绝（422），reconcile 永远卡住。
  ⭐ **「只改你管的字段」不等于「完全不碰别的」。为了让你管的字段合法而必须清理的，也是你的责任**
- 两种修法的取舍：

| <br />   | mutateFn 里只设 Path/Port（选这个） | desired 里写全默认值  |
| -------- | --------------------------- | --------------- |
| 依赖默认值    | 不依赖                         | 依赖，K8s 改默认值就又白写 |
| 别人改了探针超时 | **保留**                      | **改回**          |

选第一种：WebApp spec 没暴露超时配置，operator 只管 spec 里暴露的东西

- 容器名 `"app"` 出现三处 → lint 报 goconst → 提成 `const containerName = "app"`。只改一处的话 mutateFn 会认为容器不存在、每次重建

**3c. 409 冲突：重新入队，不当错误**

- 测试：在 envtest 里很难自然制造冲突（测试 client 直连 API server，不走缓存）→ 用 `sigs.k8s.io/controller-runtime/pkg/client/interceptor` 包一层**会撒谎的 client**，所有 `Update` 都返回 `apierrors.NewConflict(...)`
  - `interceptor.NewClient` 要 `client.WithWatch`，套件里的 `k8sClient` 是 `client.Client` → 用 `client.NewWithWatch(cfg, ...)` 另建
  - 要先改镜像制造真实变化：3b 修好后第二次 reconcile 是 `unchanged`，`CreateOrUpdate` 不会调用 Update，拦截器拦不到
  - ⭐ 断言 `updates > 0`：先确认真的走到了要测的路径，否则删掉「改镜像」后测试照样绿
- 实现：入口统一处理，一条规则只在一个地方保证（和 dedup「入口只复制一次」一样）

```go
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.reconcile(ctx, req)
	if apierrors.IsConflict(err) {
		logf.FromContext(ctx).V(1).Info("conflict, requeueing", "err", err)
		return ctrl.Result{RequeueAfter: conflictRequeueDelay}, nil
	}
	return ctrl.Result{}, err
}
```

- lint unparam 发现内层 `reconcile` 从不返回非空 Result → 签名改成只返回 `error`。内层只管做事、报告成败；要不要重试由外层决定
- `apierrors.IsConflict` 能穿过 `%w` 识别 409（`ReasonForError` 内部用 `errors.As`）。和 MISTAKES #22 正好相反：`errors.As` 能穿透 `%w`，直接类型断言不能
- ⚠️ 更正：Claude 最初建议 `ctrl.Result{Requeue: true}`，v0.25 里已 `Deprecated: Use RequeueAfter instead`。用 `RequeueAfter` 后计入 `result="requeue_after"`
- `conflictRequeueDelay = 3 * time.Second`，包级常量。理由：
  1. 正常冲突是别的 writer 刚改过对象，那次写入的事件会自动把 key 放回队列 → `RequeueAfter` 只是兜底，长一点不影响恢复速度
  2. `RequeueAfter` **绕过**限速器的指数退避；冲突持续发生（两个 controller 争同一字段，D20 小题 B1）时，延迟越短循环越快
- `V(1)` = logr 的 debug 级别（`V(0)` ≈ info，`V(1)` ≈ debug）。⚠️ 但 `cmd/main.go` 里 `zap.Options{Development: true}` 默认 Debug 级别 → **这个项目里** **`V(1)`** **默认会打印**；Warn 以上带堆栈也是它造成的。部署进集群的也是开发模式（已记进 LEARNING-PLAN 的 D18 待办）
- 不加 `conflicts_total` counter（我的决定）。代价：**持续冲突目前只能从 debug 日志看到**，关掉开发模式后就完全看不见了，到时要补上 counter
