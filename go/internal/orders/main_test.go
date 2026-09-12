//go:build integration

// 本文件让整个包共用一个 Postgres 容器（D15 §3）。
//
//	go test ./...                      # 不带标签：这一包【完全不编译】，秒级
//	go test -tags=integration ./...    # 带标签：起容器，跑真库测试
//
// ⚠️ 注意构建标签和 t.Skip 的区别：
//
//	t.Skip     测试【运行了】但什么都没做，输出是绿色的 ok —— 谎报军情
//	构建标签    文件根本没被编译，go test 明确说 "no test files"
//
// ⭐ 前者会让你以为测过了，后者不会。这是今天整节课的重点（§3.1）。
package orders_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testDSN 由 TestMain 填好，供本包所有测试使用。
//
// TODO(D15)：让 orders_test.go 里的 dsn() 返回它，
// 并给 orders_test.go 也加上 `//go:build integration` 标签。
var testDSN string

// TestMain 起一个 Postgres 容器、跑迁移、执行所有测试、清理。
//
// TODO(D15)：实现我。
//
// 需要的 import：
//
//	context, database/sql, log, os, testing, time, os
//	_ "github.com/jackc/pgx/v5/stdlib"
//	"github.com/testcontainers/testcontainers-go"
//	"github.com/testcontainers/testcontainers-go/modules/postgres"
//	"github.com/testcontainers/testcontainers-go/wait"
//
// 步骤：
//
//	① postgres.Run(ctx, "postgres:17-alpine", ...) 起容器
//	② pg.ConnectionString(ctx, "sslmode=disable") 拿 DSN，写进 testDSN
//	③ 应用 ../../migrations/001_orders.sql
//	④ code := m.Run()
//	⑤ pg.Terminate(ctx)，然后 os.Exit(code)
//
// ⚠️ 四个坑：
//
//  1. **等待策略要等两次**：
//     wait.ForLog("database system is ready to accept connections").WithOccurrence(2)
//     Postgres 启动时这行日志出现【两次】—— 第一次是初始化数据库，第二次才真就绪。
//     只等一次的话，你会连上一个马上要重启的实例，然后随机报连接错误。
//
//  2. **起容器失败要 log.Fatal，不要 Skip** —— 今天整节课的重点。
//
//  3. **整个包共用一个容器** —— 实测启动 24 秒，每个测试起一个的话
//     20 个测试就是 8 分钟。所以它在 TestMain 里，不在每个 Test 里。
//
//  4. ⭐ **os.Exit 会跳过 defer** —— 清理必须写在 m.Run() 之后、os.Exit 之前，
//     不能用 defer。这和 D14 §5「log.Fatal 会跳过 defer」是同一个坑。
//
// 起容器要一分钟左右超时：WithStartupTimeout(60*time.Second)。
func TestMain(m *testing.M) {
	startCtx, cancelStart := context.WithTimeout(context.Background(), 90*time.Second)

	pg, err := postgres.Run(
		startCtx,
		"postgres:17-alpine",
		postgres.WithInitScripts("../../migrations/001_orders.sql"),
		postgres.WithDatabase("orders"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("起 Postgres 容器: %v", err)
	}

	dsn, err := pg.ConnectionString(startCtx, "sslmode=disable")
	cancelStart()
	if err != nil {
		terminate(pg)
		log.Fatalf("获取连接串: %v", err)
	}
	testDSN = dsn

	code := m.Run()

	// ⚠️ os.Exit 会跳过 defer —— 清理必须写在 os.Exit 之前
	terminate(pg)
	os.Exit(code)
}
func terminate(pg *postgres.PostgresContainer) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := pg.Terminate(ctx); err != nil {
		log.Printf("清理容器失败（ryuk 会兜底）: %v", err)
	}
}
