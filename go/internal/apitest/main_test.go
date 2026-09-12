//go:build integration

package apitest

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

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/hzjconan/learn-program-language/go/internal/config"
	"github.com/hzjconan/learn-program-language/go/internal/server"
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

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	// ① 设置测试配置
	cfg, err := config.Load(func(k string) (string, bool) {
		switch k {
		case "DATABASE_URL":
			return testDSN, true
		case "RATE_LIMIT":
			return "100000", true // ⭐ 别让限流干扰测试
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("读取配置失败 %v", err)
	}

	// ② 建 logger， 静默log，集成测试不需要验证log内容
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// ③ 连数据库 —— 带超时的上下文
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	db, err := openDB(t, dbCtx, cfg)
	if err != nil {
		t.Fatalf("连接数据库: %v", err)
	}
	t.Cleanup(func() { db.Close() }) // ⭐ 测试结束时才关

	resetDB(t, db)

	srv := httptest.NewServer(server.NewHandler(cfg, db, logger))
	t.Cleanup(srv.Close) // ⭐ 测试结束时才关
	return srv
}

func openDB(t *testing.T, ctx context.Context, cfg config.Config) (*sql.DB, error) {
	t.Helper()

	// DATABASE_URL 是必须的
	if cfg.DatabaseURL.Reveal() == "" {
		return nil, fmt.Errorf("DATABASE_URL 未配置")
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL.Reveal())
	if err != nil {
		return nil, fmt.Errorf("创建连接池出错: %w", err)
	}

	// D13 §12：四个池参数
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(100) // = MaxOpen，避免多余连接反复建关
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// sql.Open 不建立任何连接，必须 Ping 验证
	if err := db.PingContext(ctx); err != nil {
		db.Close() //nolint:errcheck // 启动失败，关池子
		return nil, fmt.Errorf("验证数据库连通性失败: %w", err)
	}

	return db, nil
}

// resetDB 清空 public schema 下的所有表。
//
// ⭐ 不写死表名 —— 加表的时候不用改测试代码。
func resetDB(t *testing.T, testDB *sql.DB) {
	t.Helper()
	ctx := t.Context()

	rows, err := testDB.QueryContext(ctx, `
              SELECT quote_ident(schemaname) || '.' || quote_ident(tablename)
              FROM pg_tables
              WHERE schemaname = 'public'
                AND tablename <> 'schema_migrations'
      `)
	if err != nil {
		t.Fatalf("列出表: %v", err)
	}
	defer rows.Close() // D13 §4 那三行套路

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("扫描表名: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读取表名: %v", err)
	}
	if len(names) == 0 {
		return
	}

	// ⭐ 一条语句清空全部 —— 一次往返
	//nolint:gosec // 表名来自 pg_tables 且经过 quote_ident，不是外部输入
	_, err = testDB.ExecContext(ctx,
		"TRUNCATE "+strings.Join(names, ", ")+" RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("清表: %v", err)
	}
}

func terminate(pg *postgres.PostgresContainer) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := pg.Terminate(ctx); err != nil {
		log.Printf("清理容器失败（ryuk 会兜底）: %v", err)
	}
}
