# D14 §7：多阶段构建。目标是 < 30MB 的生产镜像。
#
#   cd go
#   docker build -t golearn-api .
#   docker images golearn-api
#
# TODO(D14)：这是个【骨架】—— 按讲义 §7 补完，然后验证镜像大小。

# ---------- 构建阶段 ----------
# TODO(D18) ③：多架构支持 —— docker buildx 会自动填 TARGETOS/TARGETARCH
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION

# ⭐ 先只复制依赖清单，让 go mod download 这一层能被缓存 ——
# 代码改了但依赖没改时，这层直接命中，省几十秒。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TODO(D14)：补上构建命令。要点：
#   CGO_ENABLED=0        静态链接，才能跑在 distroless/static 上
#   -ldflags "-s -w"     去掉符号表和调试信息，二进制小一大截
#   -o /out/api ./cmd/api
#
# TODO(D18) ③：改进这个构建阶段（讲义 §3、§4）
#   - ARG VERSION / ARG REVISION，用 -ldflags "-X ...buildinfo.Version=${VERSION}" 注入
#     （镜像里没有 .git，ReadBuildInfo 拿不到提交号 —— 版本和提交号都要从外面传进来）
#   - 加 -trimpath
#   - RUN --mount=type=cache,target=/root/.cache/go-build  让重复构建复用编译缓存
#   - 多架构：FROM --platform=$BUILDPLATFORM + ARG TARGETOS TARGETARCH，在构建机原生架构上交叉编译
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath \
        -ldflags "-s -w -X github.com/hzjconan/learn-program-language/go/internal/buildinfo.Version=${VERSION} -X github.com/hzjconan/learn-program-language/go/internal/buildinfo.Revision=${REVISION}" \
        -o /out/api ./cmd/api

# ---------- 运行阶段 ----------
# distroless/static 里【只有】ca 证书和时区数据，连 shell 都没有 ——
# 攻击面极小。代价是出问题不能 docker exec 进去看。
FROM gcr.io/distroless/static-debian12

COPY --from=build /out/api /api

# ⚠️ 别用 root 跑服务
USER nonroot:nonroot

EXPOSE 8080
ENTRYPOINT ["/api"]
