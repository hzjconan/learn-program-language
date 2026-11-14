# D18 ①—④ · 教学与验证记录

> 返回 [D18 笔记](../D18.md) · [讲义](../../lessons/D18.md)

2026-11-11—11-12，根据教学对话与实际命令结果整理。不代填个人思考题答案。

## ① 版本信息

`buildinfo.Get()` 先初始化注入值，再读取 debug.ReadBuildInfo；失败时保留已知信息。它读取二进制中的记录，不会在运行时执行 Git。

启动日志在连数据库之前记录 version、revision、commitTime、modified、goVersion。组装根注册 GET /version，返回 JSON，并加入限流豁免。

实际验证：完整包路径的 -X 注入 v9.9.9，启动日志和 /version 都读到该值。HTTP 200、application/json；revision 为 18246974b9ec93732cc9a6c23e0aecdd50a250ff，modified=true。

第一次启动报 DATABASE_URL 未配置。查 deploy/docker-compose.yml 后使用：

```bash
make db-up
export DATABASE_URL='postgres://devuser:devpass@localhost:5433/golearn?sslmode=disable'
/tmp/golearn-api-d18
curl -i http://localhost:8080/version
```

宿主机 5433 映射容器 5432。版本接口不查业务表，本次无需迁移表。

Review：固定结构 JSON 的 Encode 失败通常来自响应写入失败，沿用项目忽略写出错误的约定；不能再可靠地补发错误响应。此前要求必须处理编码错误的提示过于笼统，已纠正。

## ② Makefile 与交叉编译

复用 IMAGE_TAG，版本为 1824697-dirty；加入 -trimpath、-s -w 和完整包路径的 -X。

首次本机构建漏了 CGO_ENABLED=0，实际显示 1；补上后显示 0。命令里有 -X 不等于注入验证完成，还要看运行日志或接口。

| 产物 | 验证结果 |
|---|---|
| api | darwin/arm64，CGO_ENABLED=0，-trimpath=true |
| api-linux-amd64 | ELF、x86-64、statically linked、stripped |
| api-linux-arm64 | ELF、ARM aarch64、statically linked、stripped |

Linux arm64 不能直接在 Mac 上运行，即使 CPU 架构相同，操作系统的文件格式与系统调用仍不同。

Review 提醒：build、build-cross 加入 .PHONY；去掉 go version -m 后的 head -20，避免截掉构建设置。没显示 -ldflags 不能据此判断注入失败。

## ③ 镜像

增加可注入的包级 Revision：注入值优先，为空才回退自动 VCS 信息。Dockerfile 的构建阶段声明 ARG VERSION、ARG REVISION；Makefile 用 --build-arg 传入。

踩坑：声明 REVISION，却使用 ${Revision}，大小写不同导致值可能静默展开为空。已改为 ${REVISION}；没有提交号就留空，不填 unknown。

构建阶段使用 BUILDPLATFORM 原生运行，TARGETOS/TARGETARCH 指定交叉编译目标；显式 CGO_ENABLED=0，加入 -trimpath、-s -w 与 Go 编译缓存挂载。

实际运行镜像，不提供数据库配置，读到：

```text
version=1824697-dirty revision=1824697 commitTime="" modified=false goVersion=go1.26.11
fatal: 连接数据库: DATABASE_URL 未配置
```

这证明进程成功执行并读到注入值，没有验证 HTTP 服务或数据库连通性。没有 .git 且未注入时间，commitTime 留空。modified=false 是 bool 零值，不能证明工作区干净；-dirty 后缀仍表明有修改。

镜像内 Go 1.26.11，本机 Go 1.26.6：golang:1.26-alpine 标签没有锁定补丁版本。

镜像 14,565,980 字节，约 13.9MiB。原 image-size 整除后比较会放过 20.9MiB；已改为直接比较字节数，达到 20MiB（20,971,520 字节）即失败，inspect 失败也立即退出。

.dockerignore 排除 profile/、operator/。首次传输约 467MB，修改后重复构建传输约 14.78kB（增量传输量，不是完整上下文体积）。依赖层 CACHED，编译 1.8 秒；make image image-size 通过。

追问 commitTime：已讲解 git show -s --format=%cI HEAD → ARG COMMIT_TIME → -X 注入包级 CommitTime。不要用当前 date 冒充提交时间。此扩展尚未实现验证。

双平台镜像构建是选做，尚未实际验证；不能将交叉编译通过等同于双平台镜像都已运行。

## ④ operator 日志与冲突指标

目的：默认生产日志减少 Debug 输出，使用 JSON；本地通过 --zap-devel=true 恢复开发模式。冲突返回 nil 并安排重试，不计入框架错误结果；V(1) 日志关闭后，用 counter 保持可观测性。

定义无 label 的 golearn_webapp_conflicts_total，注册到 metrics.Registry，仅在 Reconcile 的 IsConflict 分支 Inc。

在 TestReconcile_ConflictIsRequeuedNotError 中比较前后差值为 1，保留拦截器实际执行、返回 nil、RequeueAfter>0 的断言。全局 counter 用差值，避免依赖累计值。

Review 发现 Development 仍为 true，随后改为 false。make test lint 通过，controller 覆盖率 96.5%，lint 0 issues；重新生成 CRD、DeepCopy 无新差异。

手动 go run ./cmd/main.go --zap-devel=true，文本日志与错误堆栈证明开关有效，但报 no matches for kind WebApp。当前集群没有 CRD；go run 不会自动安装类型定义。

用户执行 make install，CRD 创建成功，再次运行看到 Starting Controller、Starting workers，启动验证通过。Ctrl-C 后正常关闭日志也已观察到。

区分：CRD 定义类型，WebApp 是实例；make install 安装 CRD，make deploy 部署 controller。修改集群前先确认 kubeconfig context。

## ⑤ 待继续

已讲解 operator job 工作目录、Go 版本与缓存路径、make test、make lint、生成后 git diff --exit-code。尚未完成 job review、image job 或远端 Actions 验证，不能将本地测试通过记为 CI 已通过。

### 2026-11-13 · 首次 CI review

用户已补 operator 和 image 两个 job。operator 的工作目录、依赖缓存路径、通过 make test 配置 envtest、生成后 diff 的顺序正确；image 配置 PostgreSQL 并实际请求 /version、核对版本值，比仅看启动日志多验证了 HTTP 服务。

待修正：image job 又使用整除后的 MB 比较大小，应复用 Makefile 的字节检查（make image-size IMAGE=golearn-api:ci）；operator 的 setup-go 按此前要求改为 go-version-file: go/operator/go.mod；删除“子目录运行 git diff 只能看到一部分”的错误注释，无路径限制的 git diff 可检查整个工作树。建议 smoke test 也断言 revision；清理步骤名说有日志但实际只删除容器，应先打印日志。以上仅静态 review，尚未在 Actions 验证。

### 2026-11-14 · 根据 review 修改 CI

用户授权代改上述五处：operator 从 go.mod 选 Go 版本；纠正 git diff 注释；image-size 复用 Makefile 的字节检查；用 jq 断言 revision 等于 GITHUB_SHA；always 清理步骤先打印容器日志。远端 Actions 尚未验证。

### 2026-11-14 · 提交推送与 Actions 验证

提交前 make check 首次发现 strconv.ParseBool 的忽略错误违反 errcheck；改为 s.Value == "true" 并去掉 strconv 导入。随后 make check 通过，包括 lint（0 issues）、race 与 Docker 集成测试。

提交 bf27e07 已推送 origin/main。[Actions 运行 43552745295](https://github.com/hzjconan/learn-program-language/actions/runs/43552745295) 总结果 success。逐项核对 steps，test 的单元/集成测试、operator 的重新生成/diff/测试/lint、image 的构建/大小检查/版本接口冒烟测试均 success，没有跳过关键步骤。

练习⑤的首次成功运行已验证；讲义要求的“故意制造生成物过期，让 CI 红一次再恢复”实验尚未做。个人 A/B 思考题尚未作答，不将整课记为全部交作业完成。

### 2026-11-14 · 生成文件过期的故障实验

在临时分支 d18/generated-check-demo 给 WebAppSpec.HealthPath 的字段说明增加一句文字，不生成 CRD，提交 ffec387 并推送。

[故障运行 43559941032](https://github.com/hzjconan/learn-program-language/actions/runs/43559941032)：test、image 成功，operator 失败。失败日志明确来自 Check generated code is up to date：git diff --exit-code 检测到 apps.golearn.dev_webapps.yaml 中 healthPath.description 多了一行，退出码 1。因此不是环境异常，也不是“测试碰巧红了”，而是生成文件检查确实发现了漏交产物。

随后执行 make manifests generate，由工具生成 CRD 并提交 47313bd，推送触发[恢复运行 43560171591](https://github.com/hzjconan/learn-program-language/actions/runs/43560171591)。实验改动仅保留在临时分支，已切回 main，没有合并到课程主线。

本题结论：只运行生成命令只能更新 CI 的临时工作区；必须随后检查差异，才能因“仓库提交的产物已过期”而失败。字段的普通说明注释也可能进入 CRD 的 description，并非只有 validation marker 才会改变生成物。

恢复运行最终确认三个 job 全部成功：operator 1m7s、test 43s、image 47s。练习⑤的红→绿验证完成，A/B 思考题仍待用户作答。
