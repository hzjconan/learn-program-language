# Go 复习/进阶计划（3 周冲刺版）

> 面向有 Java / TypeScript 经验、Go 荒废已久的开发者
> 目标方向：**后端 Web / 微服务 API** + **云原生 / K8s 生态** + **扎实的语言基础**
> 节奏：每天 3+ 小时 × 21 天 · 教学方式：讲解 → 出题 → 你写 → 我做 code review

---

## 0. 教学约定

每天的流程固定为四步，请按这个节奏跟我配合：

| 步骤 | 谁做 | 说明 |
|---|---|---|
| 1. 讲解 | 我 | 当天知识点 + **和 Java/TS 的差异点** + 常见坑（这部分是重点，你的旧经验既是助力也是陷阱） |
| 2. 出题 | 我 | 1 道主练习 + 1~2 道小题，附带测试用例骨架 |
| 3. 编码 | 你 | 自己写，卡住了随时问我，但先别看答案 |
| 4. 评审 | 我 | 逐行 review：正确性、惯用法（idiomatic）、并发安全、性能。**这一步是真正涨功力的地方** |

**配套文件：** `lessons/` 每日讲义 · `QA.md` 随手问答手册 · `NOTES.md` 你的每日笔记 · [`MISTAKES.md`](MISTAKES.md) 错题本（D21 面试模拟会逐条重测）

**给你的三条纪律：**
1. 每道题写完先自己跑 `go vet ./...` 和 `go test -race ./...`，再交给我。
2. 不要在心里把 Go 翻译成 Java。遇到"这在 Java 里是 XXX"的念头，把它写下来问我 —— 大部分情况下 Go 的答案是"不这么干"。
3. 每天结束在 `NOTES.md` 里写 3 行：今天最反直觉的一点 / 踩的坑 / 还没搞懂的。

---

## 第 0 天：环境重建（约 1 小时，务必先做）

当前状态诊断：

- `go1.17.5 darwin/amd64`，而你的机器是 **arm64**。工具链跑在 Rosetta 转译层下，编译/测试速度腰斩。
- 1.17 太老，以下全都用不了：泛型（1.18）、`log/slog` 结构化日志（1.21）、`min`/`max`/`clear` 内置函数（1.21）、`for range` 循环变量语义修正（1.22，**这个修掉了 Go 最著名的坑**）、`http.ServeMux` 路径参数路由（1.22）、`range over func` 迭代器（1.23）、`go.work` 工作区。
- 这些不是可选糖，是现在写 Go 的默认姿势。学老版本等于学错。

**任务：**
1. 卸载旧的：`sudo rm -rf /usr/local/go`
2. 从 https://go.dev/dl/ 下载 **darwin-arm64** 的 pkg 安装最新稳定版（1.26 或更新），安装后确认输出是 `darwin/arm64`
3. 装编辑器：VS Code + `golang.go` 扩展（或 GoLand，你有 JetBrains 习惯的话）
4. 装 `golangci-lint`（一站式静态检查，相当于 ESLint + SpotBugs）
5. 在本目录初始化模块，验证跑通 hello world

验收：`go version` 显示 arm64 + 最新版；`go test` 能跑。

---

## 第 1 周：语言核心 —— 把旧习惯拆掉

> 本周主线：**Go 不是"简化版 Java"**。没有类、没有继承、没有异常、没有重载、没有构造函数。
> 每天约 1.5h 讲解+阅读，1.5h 写代码。

### D1 · 工程骨架与类型系统基础
- module / package / import path，为什么 Go 没有 Maven 坐标却也能管依赖
- 变量、`:=` 的作用域陷阱、**零值可用**哲学（对比 Java 的 `null` / TS 的 `undefined`）
- 基本类型、显式类型转换（Go 没有隐式提升）、常量与 iota
- 可见性靠首字母大小写，不是 `public`/`private`
- **练习**：温度/单位换算库 + 表驱动测试，跑通 `go test`
- 📖 讲解：[`lessons/D1.md`](lessons/D1.md) · 💻 练习：`internal/units/`

### D2 · 函数、指针、错误处理、defer
- 多返回值、命名返回值（以及为什么少用）
- **指针基础**：`*` / `&`、Go 永远传值、struct 赋值是拷贝、slice/map/chan 的真相
- **错误即值**：`error` 接口、`errors.New` / `fmt.Errorf` + `%w` 包装、`errors.Is` / `errors.As`
- 自定义错误类型；对比 Java checked exception 和 TS 的 `throw`
- `defer` 的执行时机、参数求值时机、循环里 defer 的坑
- `panic` / `recover` —— 什么时候**不该**用（90% 的情况）
- **练习**：带分层错误包装的文件解析器，要求调用方能用 `errors.As` 拿到行号
- 📖 讲解：[`lessons/D2.md`](lessons/D2.md) · 💻 练习：`internal/kvconf/`、`internal/mathx/` · 🔬 `go run ./cmd/ptrdemo`

### D3 · slice / map / string —— Bug 高发区
- 数组 vs 切片；`len` / `cap` / 底层数组共享
- `append` 何时扩容、何时**不**扩容导致意外的别名修改（这是 Go 最常见的线上事故之一）
- `copy`、切片三索引 `s[a:b:c]`
- map 的无序遍历、不可寻址、并发写直接 panic
- string 是不可变 byte 序列；`rune` vs `byte`；中文遍历的正确姿势
- **练习**：实现一个不共享底层数组的 `Filter[T]`，用测试证明它不污染入参
- 📖 讲解：[`lessons/D3.md`](lessons/D3.md) · 💻 练习：`internal/slicex/`、`internal/stringx/` · 🔬 `go run ./cmd/slicedemo`

### D4 · struct、方法、组合
- struct 定义、字面量、匿名/嵌入字段
- 方法接收者：**值 vs 指针**（何时选哪个，Go 的一致性规则）—— 在 D2 §2 指针基础上深化
- 嵌入 ≠ 继承：没有多态覆盖，只有方法提升
- 构造惯例：`NewXxx` 函数，没有构造器
- **练习**：把一段"Java 味"的继承层次（我提供）重构成 Go 的组合写法
- 📖 讲解：[`lessons/D4.md`](lessons/D4.md) · 💻 练习：`internal/payroll/` · 🔬 `go run ./cmd/embeddemo`

### D5 · interface 与泛型
- **隐式实现**：接口由使用方定义，不是实现方声明（和 Java `implements` 的根本区别）
- 小接口哲学：`io.Reader` / `io.Writer` / `error`
- 类型断言、type switch、`any`
- **nil interface != nil pointer** 的经典坑，必须彻底搞懂
- 泛型：类型参数、约束、`comparable`；什么时候该用泛型，什么时候接口更好
- **练习**：为一个数据源写可插拔抽象，用 interface 做依赖倒置 + 泛型工具函数
- 📖 讲解：[`lessons/D5.md`](lessons/D5.md) · 💻 练习：`internal/store/`、`internal/genx/` · 🔬 `go run ./cmd/ifacedemo`

### D6 · 测试与工程规范
- `go test` / 表驱动测试 / 子测试 `t.Run` / `t.Cleanup`
- `testing.B` 基准测试、`-race` 竞态检测、覆盖率
- 项目布局：`cmd/` `internal/` `pkg/` 的真实含义（`internal` 是编译器强制的）
- `go.mod` / `go.sum` / 版本选择 MVS / `go mod tidy` / vendor
- `gofmt` 不可协商、`go vet`、`golangci-lint` 配置
- **练习**：~~给前 5 天的代码补齐测试~~ → 改为**用测试抓 bug**（前 5 天的 internal/ 覆盖率已 100%，但测试都是 Claude 写的；D6 换成「我给一份 100% 覆盖却抓不到 bug 的测试，你来写能抓到的」）
- 📖 讲解：[`lessons/D6.md`](lessons/D6.md) · 💻 练习：`internal/cache/`（3 个 bug）+ `store`/`genx` 补基准与 Example

### D7 · 周综合项目 + 复盘
- **项目**：命令行日志分析工具（读取大文件 → 解析 → 聚合统计 → 多种格式输出）
- 覆盖本周全部知识点，我做一次完整的 code review
- 复盘：把你 `NOTES.md` 里的疑问集中解决
- 📖 讲解：[`lessons/D7.md`](lessons/D7.md) · 💻 项目：`internal/logx/` + `cmd/loganalyze/`

---

## 第 2 周：并发 + 服务端工程 —— 你的主战场

> 本周主线：Go 最核心的竞争力。你的 Java 并发经验（线程池、`CompletableFuture`）和 JS 的 event loop 心智模型**都要调整**。

### D8 · goroutine 与 channel
- goroutine 是什么、为什么便宜（对比 Java 平台线程 / 虚拟线程）
- channel：无缓冲 vs 有缓冲、方向类型、关闭语义、`for range` 读取
- `select`、`default`、超时模式
- 死锁、goroutine 泄漏的成因
- **练习**：用 channel 实现一个有界并发的爬取器
- 📖 讲解：[`lessons/D8.md`](lessons/D8.md) · 💻 练习：`internal/crawl/` · 🔬 `go run ./cmd/chandemo`

### D9 · sync、内存模型、竞态
- `sync.Mutex` / `RWMutex` / `WaitGroup` / `Once` / `sync.Map` 各自的适用场景
- `atomic` 包与泛型原子类型
- `errgroup`：并发任务的错误传播（生产代码里最常用的并发工具）
- Go 内存模型 & happens-before；`-race` 实战
- **"不要用共享内存来通信"** 的真实含义与边界
- **练习**：给一个故意有竞态的程序定位并修复，要求 `-race` 干净
- 📖 讲解：[`lessons/D9.md`](lessons/D9.md) · 💻 练习：`internal/racefix/`（6 个并发问题）· 🔬 `go run ./cmd/syncdemo`

### D10 · context 与并发模式
- `context.Context` 全套：取消、超时、截止时间、传值（以及为什么别拿它当 DI 容器）
- 取消信号如何贯穿整条调用链
- 经典模式：pipeline、fan-in/fan-out、worker pool、信号量限流、优雅退出
- **练习**：可取消的多阶段 pipeline，主协程取消后所有 goroutine 必须干净退出（用测试验证无泄漏）
- 📖 讲解：[`lessons/D10.md`](lessons/D10.md) · 💻 练习：`internal/pipeline/` · 🔬 `go run ./cmd/ctxdemo`

### D11 · net/http 服务端
- `http.Handler` / `HandlerFunc` / `ServeMux` 新版路由（`GET /items/{id}`）
- 中间件模式（洋葱模型，和 Express/Koa 很像，你会秒懂）
- 请求生命周期、`r.Context()`、超时的四个配置项（漏配就是线上事故）
- 优雅关闭 `Shutdown`
- 客户端：`http.Client` 复用、连接池、超时、重试
- **练习**：手写一套中间件（日志 / 恢复 / 请求 ID / 限流），不用任何框架
- 📖 讲解：[`lessons/D11.md`](lessons/D11.md) · 💻 练习：`internal/httpx/` · 🔬 `go run ./cmd/httpdemo`

### D12 · JSON、配置、结构化日志、错误分层
- `encoding/json` 的 tag、`omitempty`、自定义 `Marshaler`、数字精度坑
- 配置管理（环境变量 + 文件），十二要素应用
- **`log/slog`**：结构化日志、Handler、上下文属性（1.21 后的标准答案，别再用第三方了）
- 分层错误设计：domain error → HTTP status 的映射
- **练习**：给 D11 的服务加上配置、slog、统一错误响应
- 📖 讲解：[`lessons/D12.md`](lessons/D12.md) · 💻 练习：`internal/apperr/` + `internal/config/` · 🔬 `go run ./cmd/jsondemo`

### D13 · 数据库
- `database/sql` 接口模型、连接池参数（`SetMaxOpenConns` 等）
- 驱动选择：`pgx`（Postgres 首选）
- 事务、`context` 超时、`sql.Null*` / 指针处理 NULL
- 查询层方案对比：手写 SQL vs `sqlc`（代码生成，我推荐）vs `GORM`（ORM，Java 背景容易过度依赖，会讲清代价）
- 数据库迁移工具
- **练习**：给服务接上真实的 Postgres（Docker 起），实现 repository 层 + 事务
- 📖 讲解：[`lessons/D13.md`](lessons/D13.md) · 💻 练习：`internal/orders/` · 🔬 `make db-migrate && go run ./cmd/dbdemo`

### D14 · 周综合项目
- **项目**：一个完整的 REST API 服务 —— 分层架构（handler / service / repository）、依赖注入（构造函数注入，不需要 Spring）、完整测试、Docker 化
- 我做完整 code review：架构分层、错误处理、并发安全、可测试性

---

## 第 3 周：生产化 + 云原生

### D15 · 测试进阶
- `httptest` 测 handler、测 client
- 依赖打桩：用接口做 fake（Go 社区不爱 mock 框架，会讲为什么）
- `testcontainers-go`：真实数据库集成测试
- Fuzz 测试、golden file 测试
- **练习**：给 D14 项目补齐单测 + 集成测试，CI 里能一键跑

### D16 · 可观测性
- OpenTelemetry：trace / metric 接入
- Prometheus 指标暴露、四个黄金信号
- `net/http/pprof`：CPU / heap / goroutine profile 实操
- **练习**：故意造一个 goroutine 泄漏和一个内存泄漏，用 pprof 抓出来

### D17 · 性能与内存
- 栈 vs 堆、**逃逸分析**（`-gcflags=-m` 实操）
- GC 工作机制、`GOGC` / `GOMEMLIMIT`
- `sync.Pool`、预分配、避免不必要的分配
- benchmark 驱动优化：先测量，再优化
- **练习**：给一段慢代码做优化，要求用 benchmark 数据证明提升

### D18 · 构建与交付
- 交叉编译、`ldflags` 注入版本信息、静态链接
- 多阶段 Dockerfile → distroless / scratch 镜像（几 MB 的镜像，这是 Go 相对 JVM 的巨大优势）
- `go generate`、Makefile / Taskfile
- GitHub Actions CI 流水线
- **练习**：把 D14 项目打成 < 20MB 的生产镜像 + 完整 CI

### D19 · 云原生基础
- 为什么 K8s 生态全是 Go：读懂它们的代码风格
- **Helm**（见下方专项说明）
- `client-go`：clientset、informer、lister、workqueue 概念模型
- CRD / controller / reconcile loop 的思想（**声明式 + 最终一致**，这是核心心智模型）
- 阅读实战：带你读一段真实的 K8s controller 源码
- **练习**：用 client-go 写一个集群资源巡检 CLI

> #### 📌 Helm 专项（用户明确要求，D19 讲）
>
> **背景**：用户之前用过 Helm，但一直是「复制粘贴现成 chart 再改」，
> 没有系统学过语法，看到 `{{- if .Values.x }}` 这类东西心里没底。
>
> **定位**：⚠️ **只讲基础概念 + 语法 + 常见场景，不要展开成完整的 Helm 教程。**
> 目标是「以后能看懂任何 chart、能自己从零写一个、知道出问题去哪查」，
> 不是「精通 Helm」。预计占 D19 的三分之一。
>
> 要覆盖的：
>
> 1. **它解决什么问题** —— 和裸 `kubectl apply` / `kustomize` 的对比；
>    为什么需要模板（多环境、多副本、镜像 tag 要变）
> 2. **Chart 结构** —— `Chart.yaml` / `values.yaml` / `templates/` / `_helpers.tpl` 各是什么
> 3. **模板语法**（重点，这是他真正缺的那块）
>    - 内置对象：`.Values` / `.Release` / `.Chart` / `.Capabilities`
>    - 控制结构：`if` / `range` / `with`（讲清 `with` 会改变 `.` 的指向 —— 最常见的困惑源）
>    - 管道和常用函数：`default` / `quote` / `toYaml` / `nindent` / `required`
>    - ⭐ **空白控制 `{{-` `-}}`** —— 复制粘贴党最看不懂的就是这个，
>      而且它直接决定生成的 YAML 合不合法
>    - `define` + `include` vs `template`（为什么几乎总是用 `include`：它能接管道做 `indent`）
> 4. **values 的覆盖顺序** —— `values.yaml` < `-f custom.yaml` < `--set`
> 5. **常见场景**（每个给一个最小可用例子）
>    - 多环境（dev/staging/prod 三份 values）
>    - ConfigMap / Secret 的注入
>    - 镜像 tag 从 CI 传进来
>    - ⭐ **Hook：`pre-upgrade` 跑数据库迁移** —— 正好接 D13 §9.2 那条
>      「迁移要作为独立的部署前步骤」，把两天串起来
>    - 依赖（subchart）的基本概念，点到为止
> 6. **调试手段**（这条最实用）
>    - `helm template` —— 只渲染不安装，看生成的 YAML 到底长什么样
>    - `--dry-run --debug`、`helm lint`、`helm diff` 插件
>    - ⭐ 教学方式：**先让他改一个 chart 然后 `helm template` 看输出**，
>      比讲语法有效得多
>
> **不讲**：library chart、OCI registry 的细节、chart 签名、复杂的 chart 测试框架。

### D20 · 写一个 Operator
- `kubebuilder` 脚手架、CRD 定义、reconciler 实现
- 幂等性、错误重试、状态子资源
- 本地 `kind` 集群跑起来
- **练习**：实现一个简单但完整的 controller（比如自动给带特定注解的 Pod 注入 sidecar 配置，或管理一个自定义资源）

### D21 · 收尾、面试模拟与后续路线
- 最终项目完整评审
- **面试模拟**：我出题你答，覆盖高频考点，答完逐题点评（哪句话会让面试官追问、哪句话暴露了理解不牢）
  - 语言机制：slice 扩容与别名、map 并发、`defer` 求值时机、值/指针接收者、nil interface != nil pointer、逃逸分析
  - 并发：GMP 调度模型、channel 底层、`select` 随机性、context 取消传播、常见死锁与泄漏、`sync.Map` 适用场景
  - 运行时：GC 三色标记与写屏障、`GOGC`/`GOMEMLIMIT`、内存逃逸判断
  - 工程：错误处理设计、依赖注入、如何测不可测的代码、性能排查流程
  - 场景题：给一段有 bug 的并发代码让你找问题（这类题最能区分水平）
  - 反向准备：面试官问「你有什么问题」时该问什么
- Go 惯用法总结清单（你个人的"避坑手册"）
- 后续路线建议：gRPC / 消息队列 / 分布式系统 / 源码阅读清单
- 推荐持续跟进的资源与社区

---

## 配套资源（按优先级）

| 资源 | 用途 |
|---|---|
| [A Tour of Go](https://go.dev/tour/) | 第 0-1 天快速唤醒手感，2 小时刷完 |
| [Effective Go](https://go.dev/doc/effective_go) | **必读**，Go 惯用法圣经 |
| [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) | 我 review 你代码时的依据，建议提前扫一遍 |
| [Go by Example](https://gobyexample.com/) | 查语法的速查手册 |
| 官方标准库文档 pkg.go.dev | 日常查阅，Go 的文档质量极高 |
| 《Go 语言设计与实现》（draveness 在线免费） | 第 3 周深入运行时/GC 时配合看 |
| Go Blog + 每个版本的 Release Notes | 了解语言演进 |

---

## 里程碑验收

- **第 1 周末**：能不查资料写出符合惯用法的 Go 代码，`go vet` + lint 全绿
- **第 2 周末**：能独立设计并实现一个带数据库的生产级 HTTP 服务
- **第 3 周末**：能读懂 K8s 生态项目源码，能写出可部署的 controller

---

## 进度追踪

- [x] D0 环境重建 —— go1.26.6 darwin/arm64 · golangci-lint v2.12.2 · `make check` 全绿
- [x] D1 工程骨架与类型系统 —— `internal/units` 温度/数据量换算 · 具名类型 + iota + 表驱动测试 · `make check` 全绿
- [x] D2 函数、指针、错误处理、defer —— `internal/kvconf` 带行号的配置解析器 + `internal/mathx` recover 练习 · `make check` 全绿
- [x] D3 slice/map/string —— `internal/slicex` 不共享底层数组的切片工具 + `internal/stringx` 按 rune 的字符串处理 · `make check` 全绿
- [x] D4 struct 与组合 —— `internal/payroll` 用接口+嵌入重构 Java 继承层次 · `cmd/embeddemo` 方法集/嵌入无多态演示 · `make check` 全绿
- [x] D5 interface 与泛型 —— `internal/store` 可插拔数据源（小接口 + 依赖倒置 + nil 接口坑）+ `internal/genx` 泛型工具 · `cmd/ifacedemo` 接口二元组演示 · `make check` 全绿
- [x] D6 测试与工程规范 —— `internal/cache` 用测试抓出 3 个植入 bug（覆盖率 100% 却全漏网）· `store.CopyAll` 补测到变异 4/4 · `genx.Filter` 预分配基准对比 + `Example` 函数 · `make check` 全绿
- [x] D7 周综合项目：日志分析 CLI —— `internal/logx` 解析/聚合/可插拔输出 + `cmd/loganalyze` CLI · 覆盖率 98.7% · 变异测试 14/14 · `make check` 全绿 · **第 1 周完成**
- [x] D8 goroutine 与 channel —— `internal/crawl` 有界并发爬取器（超时不泄漏 / worker pool / 逐层去重）· 覆盖率 96.7% · 变异测试 8/8 · `cmd/chandemo` 八段演示 · `make check` 全绿
- [x] D9 sync 与竞态 —— `internal/racefix` 修复 6 个并发问题（atomic/Lock/maps.Clone/OnceValue/wg.Go）· 覆盖率 100% · 变异测试 6/6 · `-race -count=10` 干净 · 首个外部依赖 x/sync
- [x] D10 context 与并发模式 —— `internal/pipeline` 可取消的三阶段流水线（Source/Stage/Merge/Run）· 覆盖率 100% · 变异测试 8/9 · `-race -count=10` 无泄漏 · `cmd/ctxdemo` 七段演示
- [x] D11 net/http 服务端 —— `internal/httpx` 手写四个中间件（RequestID/Recover/Logging/RateLimit）+ KV API + SSE 示例 · 变异测试 9/10 · `-race` 干净 · `cmd/httpdemo` 七段演示
- [x] D12 JSON/配置/slog —— `internal/apperr` 错误分层（Kind→HTTP 映射）+ `internal/config` 环境变量加载与脱敏 · `httpx` 改造成 slog（ctxHandler 自动注入 request ID）· 变异测试 apperr 17/17 · httpx 8/8 · `cmd/jsondemo` 七个 JSON 坑 + 六段 slog
- [x] D13 数据库 —— `internal/orders` repository（事务 / rows 生命周期 / NULL / 错误翻译）· Postgres 17 via docker compose · 变异测试 7/9 · 覆盖率 87.8% · `cmd/dbdemo` 九段演示
- [ ] D14 周综合项目：REST API 服务
- [ ] D15 测试进阶
- [ ] D16 可观测性
- [ ] D17 性能与内存
- [ ] D18 构建与交付
- [ ] D19 云原生基础
- [ ] D20 Operator 实战
- [ ] D21 收尾、面试模拟与路线规划
