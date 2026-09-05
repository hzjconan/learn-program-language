.PHONY: check test race lint fmt cover clean db-up db-migrate db-psql db-down

# 交作业前跑这个。全绿再叫我 review。
check: fmt lint race

test:
	go test ./...

# -race 是竞态检测器，第 2 周开始它会救你的命。养成习惯从现在开始。
race:
	go test -race ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	golangci-lint fmt ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

clean:
	go clean -testcache
	rm -f coverage.out

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
