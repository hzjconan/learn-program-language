# D14 §7：多阶段构建。目标是 < 30MB 的生产镜像。
#
#   cd go
#   docker build -t golearn-api .
#   docker images golearn-api
#
# TODO(D14)：这是个【骨架】—— 按讲义 §7 补完，然后验证镜像大小。

# ---------- 构建阶段 ----------
FROM golang:1.26-alpine AS build
WORKDIR /src

# ⭐ 先只复制依赖清单，让 go mod download 这一层能被缓存 ——
# 代码改了但依赖没改时，这层直接命中，省几十秒。
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TODO(D14)：补上构建命令。要点：
#   CGO_ENABLED=0        静态链接，才能跑在 distroless/static 上
#   -ldflags "-s -w"     去掉符号表和调试信息，二进制小一大截
#   -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/api ./cmd/api

# ---------- 运行阶段 ----------
# distroless/static 里【只有】ca 证书和时区数据，连 shell 都没有 ——
# 攻击面极小。代价是出问题不能 docker exec 进去看。
FROM gcr.io/distroless/static-debian12

COPY --from=build /out/api /api

# ⚠️ 别用 root 跑服务
USER nonroot:nonroot

EXPOSE 8080
ENTRYPOINT ["/api"]
