// Command dbdemo 演示 database/sql 的连接池、rows 生命周期、NULL 与事务（D13）。
//
//	cd go
//	make db-migrate        # 起库 + 建表
//	go run ./cmd/dbdemo
//
// ⚠️ 这个 demo 会往 orders 表里写数据，跑完自己清理。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // 副作用导入：注册 "pgx" 驱动（§2）
)

const defaultDSN = "postgres://devuser:devpass@localhost:5433/golearn?sslmode=disable"

func dsn() string {
	if v, ok := os.LookupEnv("DB_DSN"); ok && v != "" {
		return v
	}
	return defaultDSN
}

func main() {
	ctx := context.Background()

	db := open(ctx)
	defer db.Close() //nolint:errcheck // demo 退出即结束

	openDoesNotConnect()
	autoCloseVsEarlyExit(ctx)
	noGCSafetyNet(ctx)
	poolExhaustion(ctx)
	rowsErrIsSilent(ctx)
	nullHandling(ctx, db)
	queryRowErrors(ctx, db)
	transactionRollback(ctx, db)
	poolStats(ctx)
}

func open(ctx context.Context) *sql.DB {
	db, err := sql.Open("pgx", dsn())
	if err != nil {
		fmt.Fprintf(os.Stderr, "sql.Open: %v\n", err)
		os.Exit(1)
	}
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "连不上数据库: %v\n\n先跑：make db-migrate\n", err)
		os.Exit(1)
	}
	return db
}

// newDB 每段演示用独立的池，免得互相干扰。
//
// sql.Open 在这里不可能失败（DSN 上面已经用同一个字符串 Ping 通了），
// 但仓库的 errcheck 开了 check-blank，`db, _ :=` 会被挡下来 ——
// 与其加 nolint，不如老实处理（D12 demo 里那条 Must 惯用法）。
func newDB(maxOpen int) *sql.DB {
	db, err := sql.Open("pgx", dsn())
	must(err)
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
	return db
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---------- ① sql.Open 不连接 ----------

func openDoesNotConnect() {
	sec("① sql.Open 不连接数据库")

	bad, err := sql.Open("pgx", "postgres://nobody:nope@127.0.0.1:1/nodb?sslmode=disable")
	fmt.Printf("  sql.Open 一个【完全连不上】的 DSN → err = %v\n", err)
	fmt.Println("  ⭐ 它只解析 DSN、准备好池，一个网络包都不发。")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	fmt.Printf("  PingContext → %s\n", short(bad.PingContext(ctx)))
	fmt.Println("  ⚠️ 启动时必须 Ping，否则第一个真实请求才发现连不上。")
	bad.Close() //nolint:errcheck
}

// ---------- ② rows 读到底 vs 提前退出 ----------

func autoCloseVsEarlyExit(ctx context.Context) {
	sec("② rows：读到底会自动关，提前退出不会")

	db := newDB(4)
	defer db.Close() //nolint:errcheck

	// 读到底
	rows, err := db.QueryContext(ctx, `SELECT generate_series(1,3)`)
	if err != nil {
		fmt.Println("  query:", err)
		return
	}
	for rows.Next() {
		var n int
		_ = rows.Scan(&n) //nolint:errcheck
	}
	fmt.Printf("  Next() 返回 false 之后（没调 Close）: InUse=%d Idle=%d   ✅ 自动关了\n",
		db.Stats().InUse, db.Stats().Idle)

	// 只读一行就走 —— 模拟 break / return / Scan 出错
	rows2, err := db.QueryContext(ctx, `SELECT generate_series(1,3)`)
	if err != nil {
		fmt.Println("  query:", err)
		return
	}
	rows2.Next()
	fmt.Printf("  只读一行就不管了（break/return）:      InUse=%d Idle=%d   ⚠️ 连接被占住\n",
		db.Stats().InUse, db.Stats().Idle)
	rows2.Close() //nolint:errcheck

	fmt.Println()
	fmt.Println("  ⭐ happy path 不写 Close 也没事 —— 这正是危险之处：")
	fmt.Println("     测试全绿，泄漏藏在你不会去测的 error / break 分支里。")
	fmt.Println("     所以 defer rows.Close() 不是可选的。")
}

// ---------- ③ 没有 GC 兜底 ----------

func noGCSafetyNet(ctx context.Context) {
	sec("③ 泄漏的 rows 不会被 GC 回收")

	db := newDB(5)
	defer db.Close() //nolint:errcheck

	for range 3 {
		rows, err := db.QueryContext(ctx, `SELECT generate_series(1,3)`)
		if err != nil {
			fmt.Println("  query:", err)
			return
		}
		_ = rows // 立刻失去引用
	}
	fmt.Printf("  三次查询都不关，也不再引用:  InUse=%d Open=%d\n",
		db.Stats().InUse, db.Stats().OpenConnections)

	runtime.GC()
	time.Sleep(150 * time.Millisecond)
	runtime.GC()
	time.Sleep(150 * time.Millisecond)

	fmt.Printf("  两次 runtime.GC() 之后:      InUse=%d Open=%d   ⚠️ 纹丝不动\n",
		db.Stats().InUse, db.Stats().OpenConnections)
	fmt.Println("  ⭐ database/sql 没给 Rows 设 finalizer。连接被永久占住，直到进程退出。")
}

// ---------- ④ 池耗尽 ----------

func poolExhaustion(ctx context.Context) {
	sec("④ 后果：整个服务挂住")

	db := newDB(2)   // 池上限 2
	defer db.Close() //nolint:errcheck

	var held []*sql.Rows // ⭐ 持有引用，模拟「泄漏还没被回收」
	var cancels []context.CancelFunc

	// ⚠️ 注意这里【没有】在循环里调 cancel() —— 取消 ctx 会让 database/sql
	// 关掉 rows 并归还连接，那就演示不出泄漏了。（我写这个 demo 时踩了两次。）
	for i := 1; i <= 3; i++ {
		c, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		cancels = append(cancels, cancel)

		start := time.Now()
		rows, err := db.QueryContext(c, `SELECT generate_series(1,3)`)
		el := time.Since(start).Round(time.Millisecond)

		if err != nil {
			fmt.Printf("  第 %d 次: ⚠️ 等了 %v 后失败 —— %s\n", i, el, short(err))
			break
		}
		held = append(held, rows)

		flag := ""
		if el > 100*time.Millisecond {
			flag = "   ← ⚠️ 在排队等连接"
		}
		fmt.Printf("  第 %d 次: ok（%v）  InUse=%d%s\n", i, el, db.Stats().InUse, flag)
	}
	for _, r := range held {
		r.Close() //nolint:errcheck
	}
	for _, c := range cancels {
		c()
	}

	fmt.Println()
	fmt.Println("  ⭐ 第 3 次【没有报错，它在等】。生产上 MaxOpenConns 是 25/50，")
	fmt.Println("     泄漏几十次之后所有请求一起排队，表现是「整个服务同时变慢再挂死」，")
	fmt.Println("     而且日志里往往什么都没有 —— 大家都在等，没人报错。")
}

// ---------- ⑤ rows.Err() ----------

func rowsErrIsSilent(ctx context.Context) {
	sec("⑤ 忘记 rows.Err()：静默丢数据")

	db := newDB(2)
	defer db.Close() //nolint:errcheck

	// 这个查询在【第 5 万行】才除零。前面的行早就成批返回了，
	// 所以错误不可能在 Query 阶段暴露 —— 只能在后续的 Next() 里冒出来。
	//
	// ⚠️ 对比：如果错误发生在第 1 行，Query 阶段就报错了，压根进不到循环。
	// 真实世界里「迭代中途才失败」的原因是连接断了 / 上游超时 / 网络抖动，
	// 这些在测试里几乎不会发生 —— 所以这个 bug 极难被测出来。
	const q = `SELECT 1/(50000-i) FROM generate_series(1,60000) i`

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		fmt.Printf("  Query 阶段就报错了: %s\n", short(err))
		return
	}
	defer rows.Close() //nolint:errcheck

	n, scanErr := 0, error(nil)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			scanErr = err
			break
		}
		n++
	}

	fmt.Printf("  想读 60000 行，循环「正常」结束，实际读到 %d 行\n", n)
	fmt.Printf("  循环体里的 Scan 错误: %v   ⚠️ 一个都没有\n", scanErr)
	fmt.Printf("  rows.Err()          : %s   ← 只有它知道\n", short(rows.Err()))
	fmt.Println()
	fmt.Println("  ⭐ 不查 rows.Err()，你会拿着这 49999 行当成全部 60000 行继续往下跑。")
	fmt.Println("     没有 panic、没有错误日志、没有任何征兆。")
}

// ---------- ⑥ NULL ----------

func nullHandling(ctx context.Context, db *sql.DB) {
	sec("⑥ NULL 的四种接法")

	if _, err := db.ExecContext(ctx,
		`INSERT INTO orders (customer, note) VALUES ('demo-null', NULL)`); err != nil {
		fmt.Println("  insert:", err)
		return
	}
	defer db.ExecContext(ctx, `DELETE FROM orders WHERE customer = 'demo-null'`) //nolint:errcheck

	const q = `SELECT %s FROM orders WHERE customer='demo-null' LIMIT 1`

	var s string
	err := db.QueryRowContext(ctx, fmt.Sprintf(q, "note")).Scan(&s)
	fmt.Printf("  扫进 string           → %s\n", short(err))

	var ns sql.NullString
	err = db.QueryRowContext(ctx, fmt.Sprintf(q, "note")).Scan(&ns)
	fmt.Printf("  扫进 sql.NullString   → err=%v Valid=%v String=%q\n", err, ns.Valid, ns.String)

	var p *string
	err = db.QueryRowContext(ctx, fmt.Sprintf(q, "note")).Scan(&p)
	fmt.Printf("  扫进 *string          → err=%v p=%v\n", err, p)

	var c string
	err = db.QueryRowContext(ctx, fmt.Sprintf(q, "COALESCE(note,'')")).Scan(&c)
	fmt.Printf("  SQL 里 COALESCE       → err=%v c=%q   ⭐ 最省事\n", err, c)

	fmt.Println()
	fmt.Println("  ⭐ 能在 SQL 里 COALESCE 就 COALESCE —— 把「可选」挡在数据库边界，")
	fmt.Println("     Go 这边结构体保持干净的值类型。只有当 NULL 和零值【语义不同】时，")
	fmt.Println("     才让它渗进 Go 类型里（和 D12 §1.2 是同一个判断）。")
}

// ---------- ⑦ QueryRow 的错误 ----------

func queryRowErrors(ctx context.Context, db *sql.DB) {
	sec("⑦ QueryRow 的错误延迟到 Scan")

	var name string
	err := db.QueryRowContext(ctx, `SELECT customer FROM orders WHERE id = -1`).Scan(&name)
	fmt.Printf("  查不到时 Scan 返回: %v\n", err)
	fmt.Printf("  errors.Is(err, sql.ErrNoRows) = %v\n", errors.Is(err, sql.ErrNoRows))
	fmt.Println()
	fmt.Println("  ⚠️ QueryRow 本身【不返回 error】—— 忽略 Scan 的返回值 = 忽略全部错误。")
	fmt.Println("  ⭐ sql.ErrNoRows 通常不是「错误」，是「没找到」：")
	fmt.Println("     在 repository 层翻译成 apperr.NotFound（D12 §5），别让它漏到 handler。")
}

// ---------- ⑧ 事务回滚 ----------

func transactionRollback(ctx context.Context, db *sql.DB) {
	sec("⑧ 事务：一步失败，整体回滚")

	before := countOrders(ctx, db)

	err := func() (err error) { // ⭐ 命名返回值，defer 才能改它
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		// ⭐ 无脑 Rollback：Commit 过了就返回 ErrTxDone，忽略即可
		defer func() {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, rbErr)
			}
		}()

		var id int64
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO orders (customer) VALUES ('demo-tx') RETURNING id`).Scan(&id); err != nil {
			return err
		}
		fmt.Printf("  订单插入成功，id=%d（事务内可见）\n", id)

		// 故意违反 CHECK (qty > 0)
		_, err = tx.ExecContext(ctx,
			`INSERT INTO order_items (order_id, sku, qty, price_cents) VALUES ($1,'X',0,100)`, id)
		if err != nil {
			fmt.Printf("  订单项插入失败: %s\n", short(err))
			return err
		}
		return tx.Commit()
	}()

	fmt.Printf("  函数返回的错误: %s\n", short(err))
	fmt.Printf("  事务前订单数=%d，事务后=%d   ⭐ 回滚了，那条订单不存在\n",
		before, countOrders(ctx, db))
	fmt.Println()
	fmt.Println("  ⚠️ 事务里所有操作必须用 tx. 而不是 db. ——")
	fmt.Println("     db. 会从池里另拿一条连接，那条语句【不在这个事务里】，回滚也撤不掉它。")
}

func countOrders(ctx context.Context, db *sql.DB) int {
	var n int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM orders`).Scan(&n) //nolint:errcheck
	return n
}

// ---------- ⑨ 池参数 ----------

func poolStats(ctx context.Context) {
	sec("⑨ 连接池参数的可见效果")

	db := newDB(3)
	defer db.Close()      //nolint:errcheck
	db.SetMaxIdleConns(1) // ⚠️ 故意设小，看它的代价

	done := make(chan struct{})
	for range 6 {
		go func() {
			defer func() { done <- struct{}{} }()
			var n int
			_ = db.QueryRowContext(ctx, `SELECT pg_sleep(0.15), 1`).Scan(new(any), &n) //nolint:errcheck
		}()
	}
	time.Sleep(60 * time.Millisecond)
	st := db.Stats()
	fmt.Printf("  MaxOpen=3，6 个并发查询进行中:\n")
	fmt.Printf("    Open=%d InUse=%d Idle=%d WaitCount=%d\n",
		st.OpenConnections, st.InUse, st.Idle, st.WaitCount)

	for range 6 {
		<-done
	}
	st = db.Stats()
	fmt.Printf("  全部结束后:\n")
	fmt.Printf("    Open=%d Idle=%d WaitCount=%d WaitDuration=%v\n",
		st.OpenConnections, st.Idle, st.WaitCount, st.WaitDuration.Round(time.Millisecond))
	fmt.Println()
	fmt.Println("  ⭐ WaitCount 是「有几次查询排过队」，WaitDuration 是累计等待时间。")
	fmt.Println("     这两个涨得快 = 池不够用，或者有人在泄漏连接。把它们暴露成 metric（D16）。")
	fmt.Println("  ⚠️ MaxIdleConns=1，所以结束后只剩 1 条空闲，其余用完就关 ——")
	fmt.Println("     高并发下会反复建连。生产上 MaxIdleConns 应该等于 MaxOpenConns。")
}

// ---------- 辅助 ----------

func sec(s string) { fmt.Printf("\n=== %s ===\n%s\n", s, strings.Repeat("-", 62)) }

// short 把多行/超长的错误压成一行，方便对齐输出。
func short(err error) string {
	if err == nil {
		return "<nil>"
	}
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i]
	}
	if len(s) > 96 {
		s = s[:96] + "…"
	}
	return s
}
