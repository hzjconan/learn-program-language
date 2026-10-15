# D16 ⑥ OTel trace

> 返回 [D16 笔记](../D16.md) · 讲义 [lessons/D16.md](../../lessons/D16.md) §8

（2026-10-08。golearn-api 接入 OTel：每个请求一条 trace，日志带 `trace_id`）

| 步骤 | 做了什么                                                | 改了哪里                                          |
| -- | --------------------------------------------------- | --------------------------------------------- |
| 1  | 装依赖，实现 `SetupTracing`                               | `internal/obs/tracing.go`                     |
| 2  | `run()` 里启动/关闭 tracing；`otelhttp` 包在最外层             | `cmd/api/main.go`、`internal/server/server.go` |
| 3  | `ordersvc.Place` / `Pay` 加 span；日志带 `trace_id` + 测试 | `internal/ordersvc`、`internal/httpx`          |
| 4  | 跑起来验证：日志和 span 对上；跨服务传播                             | —                                             |

**1. 依赖和** **`SetupTracing`**

- `go get` 要 `env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy ...`（shell 里的代理平时不在跑）
- 四个包对应 API / SDK 分离（讲义 §7.2）：

| 包             | 作用                           | 类比              |
| ------------- | ---------------------------- | --------------- |
| `otel`        | API：业务代码开 span               | SLF4J           |
| `otel/sdk`    | SDK：记录、攒批、导出                 | logback         |
| `stdouttrace` | exporter：span 打到 `io.Writer` | ConsoleAppender |
| `otelhttp`    | HTTP 中间件，每个请求自动一个 span       | —               |

- `go get` 顺带把已有的间接依赖升级了（`otel` 1.44 → 1.47、`otelhttp` 0.69 → 0.72、`testify`、`x/sys`…），testcontainers、client-go 也跟着用新版本。跑了完整 `make check` 确认没被影响。**只想加一个功能，结果把别人依赖的版本也改了**，是升级依赖的常见风险
- `WithBatcher` 不阻塞请求，代价是**退出前必须 shutdown**，否则最后一批 span 丢失
- 错误加上本层上下文：`fmt.Errorf("create stdout trace exporter: %w", err)`
- `ctx` 参数现在没用到，留着是因为换成 `otlptracegrpc.New(ctx, ...)` 时要用（连 collector 的超时和取消），注释里写明了，免得被当成多余参数删掉

**2. 接进** **`run()`** **和** **`NewHandler`**

- span 打到 `os.Stderr`、日志在 `os.Stdout`，验证时可以 `> logs 2> spans` 分开
- shutdown：`defer` + 新 `context.Background()` + `cfg.ShutdownTimeout`，失败记日志「关闭 tracing 失败，最后一批 span 可能丢失」
- `otelhttp.NewHandler(httpx.Chain(...), "golearn-api")` 包在**最外层**：后面所有中间件（包括 Logging）拿到的 ctx 里都已经有 span
- **为什么用** **`context.Background()`**：我的答案：trace 和 db、关 server 没关系。方向对，更具体的理由是**生命周期**：
  - `dbCtx` 10 秒超时，专给连数据库，用完就取消
  - signal 的 `ctx` 一收到 SIGTERM 就取消。换成 OTLP exporter 后它维护到 collector 的连接，一收到退出信号就断了，**shutdown 想刷最后一批 span 时已经发不出去**，而那正是 Pod 被停前最后几秒的 span
  - tracing 需要比 signal **活得更久**的 ctx。和 D14「`Shutdown` 要用新的 `Background()`」同一个道理，只是这次在启动时
- **defer 顺序**：我的答案：后进先出，trace 要最后执行，所以写在最上面 ✅。为什么这个顺序对：
  `serve()` 要等 HTTP 服务**完全关闭**、在途请求都处理完才返回，此时所有请求的 span 都已结束、进了 Batcher 缓冲区；之后 defer 才执行，tracing 放最后保证一个不漏刷出去
- 错误消息 `"设置 tracing 失败: %w"` → `"设置 tracing: %w"`，和同函数里 `"加载配置: %w"`、`"连接数据库: %w"` 一致（D2 §4：「失败」是多余的）

**3a.** **`ordersvc`** **的 span**

最终写法（命名返回值 + defer，所有返回路径统一处理）：

```go
func (s *Service) Pay(ctx context.Context, id int64) (err error) {
	ctx, span := otel.Tracer("ordersvc").Start(ctx, "ordersvc.Pay",
		trace.WithAttributes(attribute.Int64("order.id", id)))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	...
}
```

第一版的四个问题（测试一个都抓不到，测试不看 span 内容）：

1. ❌ **`Pay`** **的 span 名字是** **`ordersvc.Place`**：从 Place 复制后没改。上线后每次支付在 trace 里都显示成下单。错误的观测数据比没有更糟，会把排查的人带偏（review 关注点「不撒谎」）
2. ❌ **只有** **`RecordError`，没有** **`SetStatus(codes.Error, ...)`**：`RecordError` 只是挂一个事件，span 状态还是 Unset，后端当成功显示，「只看出错的 trace」也筛不出来
3. ❌ **8 个 return 前各写一次** **`RecordError`，还是漏了 3 条**：`repo.Create` 失败、`Pay` 的状态转换 Conflict、`repo.UpdateStatus` 失败。偏偏数据库失败最需要看到。
   ⭐ 今天第三次同类问题（dedup 两处写入、Reconcile 三处 409）：**多条路径做同一件事，逐条处理总会漏一条 → 在一个地方统一处理**。这次用命名返回值 + defer
4. ❌ **`Pay`** **的** **`order.id`** **设得太晚**（所有检查通过后才设）：失败的支付恰好没有订单号。`id` 是参数，`Start` 时就用 `trace.WithAttributes` 设上

重构后又丢了一处：`Place` 改成 `return s.repo.Create(ctx, o)` 时，成功后设置 `order.id` 的代码也一起删了 → 补进 defer 的 `else` 分支（成功时 `o.ID` 已由 `repo.Create` 填好）。重构时注意力在「集中错误处理」上，顺手删了一段和错误无关的代码。

**命名返回值 + defer 的语义**（写了小程序验证）：

> Go 规范：A "return" statement that specifies results sets the result parameters before any deferred functions are executed.

| 写法                                                   | defer 看到的 `err` |
| ---------------------------------------------------- | --------------- |
| `return errors.New("conflict")`（不写 `err :=`，直接返回表达式） | `conflict` ✅    |
| 块里 `:=` 新声明 `err`，错误存进去但**没有 return**                | `<nil>` ❌       |
| 块里 `:=` 新声明 `err`，然后 `return err`                    | `db down` ✅     |

- 直接 `return 表达式` 完全没问题：先赋给命名返回值，再执行 defer
- 要提防的不是 `:=` 遮蔽本身，而是「**错误存进了变量却没被 return**」（这本身就是吞错误的 bug，有 defer 时更难察觉，因为 span 显示成功）
- 在遮蔽了 `err` 的块里写裸 `return`，编译器直接报 `err is shadowed during return`

**3b/3c. 日志带** **`trace_id`** **+ 测试**

```go
// httpx.ctxHandler.Handle，和 request_id 并列
if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
	r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
}
```

`TestHTTPX_LogsTraceID` 两个子测试：

- ctx 里有 span：`trace_id` **等于** `sc.TraceID().String()`（比较具体值，不只是字段存在：防止把 span\_id 当 trace\_id 写进去）
- ctx 里没有 span：日志里**不出现** `trace_id` 字段。去掉 `IsValid()` 判断会打出全零 `00000000…`，查不到任何东西，按 trace\_id 分组时所有无 span 的日志还会被归成一组。
  断言「字段不存在」（`_, ok := ln["trace_id"]`），不是「值为空」：和 ③「判断序列不存在」同一类，**「没有」和「有但为空」是两种状态**
- 不需要装 SDK：`trace.NewSpanContext(...)` + `trace.ContextWithSpanContext(...)` 直接构造。只构造被测代码需要的那部分输入（和 request\_id 测试里 `context.WithValue(ctx, reqIDKey, "req-1")` 一样）。`TraceID`、`SpanID` 都必须非零，否则 `IsValid()` 是 false
- 红绿验证（Claude 用 overlay）：去掉 `IsValid()` → 第二个子测试红，报出全零 trace\_id

**4. 跑起来验证**

`make db-migrate`，然后 `go run ./cmd/api > profile/d16/api-logs.jsonl 2> profile/d16/api-spans.json`，下单、支付，`Ctrl-C` 后再看文件（Batcher 约 5 秒导一批，shutdown 才把最后几个刷出来）。

⚠️ **第一次跑：日志里没有** **`trace_id`，连** **`request_id`** **都没有**

```
level=INFO msg=请求完成 method=POST path=/orders/1/pay status=200 bytes=168 duration_ms=7.94325
```

原因：**从 D14 起，生产代码里从来没有调用过** **`httpx.WithRequestID`**。`config.NewLogger` 返回的是 `slog.New(h)`，没包这一层；全仓库唯一出现它的地方是 `middleware.go` 里一行**注释**。
D14 `run()` 的 TODO 写的是「`slog + config.NewLogger + httpx.WithRequestID`」，最后一步漏了。

- 这一个月 golearn-api 所有日志都没有 `request_id`，D11、D12 设计的请求关联**从来没在线上生效过**
- 测试都过：`httpx` 的测试都是**自己**组装 logger，测的是 `ctxHandler` 能不能工作（能）；没有测试检查 `run()` 组装出来的那个 logger 有没有用它
- 和 ③「gauge 没注册」同一类：**零件是好的，测试只测了零件，但它没装到真正上线的那台机器上**。
  D14 的原则「组装放在 `internal/server`，集成测试验证真正上线的那份组装」没覆盖到 logger，它留在了 `main` 里
- 修复（在 `main` 组装，`config` 是底层包，不该依赖 `httpx`）：

```go
baseLogger := config.NewLogger(cfg, os.Stdout)
logger := slog.New(httpx.WithRequestID(baseLogger.Handler()))
```

修好后：

```
msg=请求完成 method=POST path=/orders/1/pay status=200 ... request_id=ef1da24aae8eec6ec07fdce8459f4576 trace_id=b169755a90f1ef542a3c83823150dead
```

**日志和 span 对上**（第 6 步）：

| 支付请求                        | `trace_id` / `TraceID`               | 父 span                   |
| --------------------------- | ------------------------------------ | ------------------------ |
| 日志「请求完成 path=/orders/1/pay」 | `b169755a90f1ef542a3c83823150dead`   | —                        |
| span `POST`                 | `b169755a90f1ef542a3c83823150dead` ✅ | 无（根）                     |
| span `ordersvc.Pay`         | `b169755a90f1ef542a3c83823150dead` ✅ | `89c371ce…` = 上面的 `POST` |

- 拿着日志里的 `trace_id` 去 trace 后端就能看到完整调用树：HTTP 层 7.9ms，其中 `ordersvc.Pay` 7.1ms（讲义 §7.4）
- 父子关系正确 → 3a 的 `ctx, span :=` 把新 ctx 传下去了。`Place` 的 span 上有 `order.id = 1` ✅
- `request_id` 和 `trace_id` 是两个独立生成的不同值（小题 B3 可以拿来当例子）
- ⚠️ HTTP span 的名字只有 `"POST"`，下单和支付在名字上分不出来；`url.path` 又是 `/orders/1/pay` 这种带具体 ID 的值。
  原因：`otelhttp` 在最外层，创建 span 时还没经过 mux 路由，`r.Pattern` 是空的（讲义 §2.4、QA）。
  可选改进：mux 里层加个小中间件 `trace.SpanFromContext(r.Context()).SetName(r.Method + " " + r.Pattern)`，或每个路由包 `otelhttp.WithRouteTag`

**跨服务传播**（第 7 步）：`curl ... -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'`

我的预测：`trace_id` 是 `4bf92f…`，`Remote` 是 true ✅

| <br />          | 设了 propagator                     | 注释掉 propagator       |
| --------------- | --------------------------------- | -------------------- |
| 日志 `trace_id`   | `4bf92f35…`，和上游一致                 | `ee751b34…`，**新生成的** |
| span `TraceID`  | `4bf92f35…`                       | `ee751b34…`          |
| span 的 `Parent` | `SpanID 00f067aa…`，`Remote: true` | 全零                   |
| HTTP 响应         | 200                               | **还是 200**           |
| 报错              | 没有                                | **也没有**              |

```
设了：  "Name":"GET","SpanContext":{"TraceID":"4bf92f3577b34da6a3ce929d0e0e4736",...,"Remote":false},
        "Parent":{"TraceID":"4bf92f3577b34da6a3ce929d0e0e4736","SpanID":"00f067aa0ba902b7",...,"Remote":true}
没设：  "Name":"GET","SpanContext":{"TraceID":"ee751b34283b694ad9d0e10b7c395f35",...},
        "Parent":{"TraceID":"00000000000000000000000000000000","SpanID":"0000000000000000",...}
```

- 设了：`otelhttp` 从请求头读出 `traceparent`，沿用上游 trace\_id，把上游 span 当父 span → golearn-api 这段接在上游那条 trace 下面
- `Parent.Remote: true` = 父 span 来自**另一个进程**（从请求头读来的）；自己的 `SpanContext.Remote: false` = 本进程创建的
- 没设：`traceparent` **被直接忽略**，自己开一条新 trace，成了孤立的根。trace 后端里是两条毫不相干的 trace
- ⭐ **状态码照样 200，日志里照样有一个看起来正常的** **`trace_id`，没有任何地方报错**（讲义 §7.3）。只有去 trace 后端从上游往下追时，才发现链路在这里断了
- 验证完已恢复那一行

***
