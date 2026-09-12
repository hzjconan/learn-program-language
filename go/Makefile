.PHONY: check test test-all race race-all lint fmt cover clean clean-containers \
        db-up db-migrate db-psql db-down

# D15 起，带 //go:build integration 标签的测试（testcontainers、真库）
# 要显式加标签才会跑。
TAGS = -tags=integration

# ---------- 日常 ----------

# 交作业前跑这个。全绿再叫我 review。
#
# ⭐ 它跑【全部】测试，含集成测试（要 Docker）。
# 默认值落在「最吵闹」的那一档 —— 和 D12 §5「Kind 零值选 KindInternal」
# 同一条原则：⚠️ 默认不该是「悄悄少跑一批」。
check: fmt lint race-all

# 快路径：不带标签，秒级。改代码时的内循环用这个。
#
# ⚠️ 输出里 internal/orders 和 internal/apitest 会显示 [no test files] ——
# 那是正常的，它们被构建标签隔离了。想跑就用 test-all。
test:
	go test ./...

# 全部测试，含 testcontainers 起真库的那些。
test-all:
	go test $(TAGS) ./...

# -race 是竞态检测器，第 2 周开始它会救你的命。养成习惯从现在开始。
race:
	go test -race ./...

race-all:
	go test -race $(TAGS) ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	golangci-lint fmt ./...

# ⚠️ 覆盖率必须带标签统计。
# 不带的话 internal/orders 压根没编译进来，报出的数字是【假的】——
# 它不是「覆盖率低」，是「这个包没参与」。
cover:
	go test $(TAGS) -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

clean:
	go clean -testcache
	rm -f coverage.out

# 清掉 testcontainers 残留的容器。
#
# 正常路径由 TestMain 里的 Terminate 清；测试 panic / Ctrl-C 时
# 靠 ryuk 兜底，但它有约 10 秒宽限期（QA #20）。急着重跑就用这个。
clean-containers:
	@docker ps -aq --filter label=org.testcontainers=true | xargs -r docker rm -f 2>/dev/null || true
	@echo "testcontainers 残留已清理"

# ---------- D13 数据库 ----------

DB_DSN ?= postgres://devuser:devpass@localhost:5433/golearn?sslmode=disable
COMPOSE = docker compose -f deploy/docker-compose.yml

db-up:
	$(COMPOSE) up -d
	@until docker exec golearn-pg pg_isready -U devuser -d golearn >/dev/null 2>&1; do sleep 1; done
	@echo "Postgres 就绪：$(DB_DSN)"

db-migrate: db-up
	docker exec -i golearn-pg psql -U devuser -d golearn -q < migrations/001_orders.sql
	@echo "表已重建"

db-psql:
	docker exec -it golearn-pg psql -U devuser -d golearn

db-down:
	$(COMPOSE) down -v
