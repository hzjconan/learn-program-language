package orders_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
	"github.com/hzjconan/learn-program-language/go/internal/orders"
)

const defaultDSN = "postgres://devuser:devpass@localhost:5433/golearn?sslmode=disable"

func dsn() string {
	if v, ok := os.LookupEnv("DB_DSN"); ok && v != "" {
		return v
	}
	return defaultDSN
}

// openDB 打开一个连接池；连不上就 skip 整个测试。
//
// ⚠️ Skip 不是「通过」。交作业前确认输出里【没有】 SKIP ——
// 数据库没起的话这一整包等于没测。
func openDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", dsn())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close() //nolint:errcheck
		t.Skipf("连不上数据库（%v）——先跑 `make db-migrate`", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return db
}

// freshRepo 返回一个连着【干净表】的 Repo。
func freshRepo(t *testing.T) (*orders.Repo, *sql.DB) {
	t.Helper()
	db := openDB(t)
	truncate(t, db)
	return orders.NewRepo(db), db
}

func truncate(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`TRUNCATE order_items, orders RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("清表失败（表建了吗？先跑 make db-migrate）: %v", err)
	}
}

func sampleOrder() *orders.Order {
	return &orders.Order{
		Customer: "alice",
		Note:     "尽快发货",
		Items: []orders.Item{
			{SKU: "A-1", Qty: 2, PriceCents: 500},
			{SKU: "B-2", Qty: 1, PriceCents: 1250},
		},
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	//nolint:gosec // table 是测试里写死的常量，不是外部输入
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// ---------- Create ----------

func TestOrders_CreateHappyPath(t *testing.T) {
	repo, db := freshRepo(t)
	ctx := context.Background()

	o := sampleOrder()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if o.ID == 0 {
		t.Error("Create 之后 o.ID 还是 0 —— 要用 RETURNING id 把主键写回来")
	}
	if o.CreatedAt.IsZero() {
		t.Error("Create 之后 o.CreatedAt 还是零值 —— RETURNING created_at 写回来")
	}
	// ⭐ 金额由服务端算：2*500 + 1*1250 = 2250
	if o.TotalCents != 2250 {
		t.Errorf("TotalCents = %d, want 2250（sum(qty*price)，服务端自己算）", o.TotalCents)
	}
	for i, it := range o.Items {
		if it.ID == 0 {
			t.Errorf("Items[%d].ID 还是 0 —— item 的主键也要写回来", i)
		}
	}
	if n := countRows(t, db, "orders"); n != 1 {
		t.Errorf("orders 表里有 %d 行, want 1", n)
	}
	if n := countRows(t, db, "order_items"); n != 2 {
		t.Errorf("order_items 表里有 %d 行, want 2", n)
	}
}

// TestOrders_CreateIgnoresClientTotal 锁住「金额不信调用方」。
//
// ⚠️ 这是个安全问题，不只是数据一致性：如果直接采信请求里的 total，
// 客户端可以下单 100 件商品却只付 1 分钱。
func TestOrders_CreateIgnoresClientTotal(t *testing.T) {
	repo, _ := freshRepo(t)

	o := sampleOrder()
	o.TotalCents = 1 // 客户端谎报
	if err := repo.Create(context.Background(), o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if o.TotalCents != 2250 {
		t.Errorf("TotalCents = %d, want 2250\n"+
			"（⚠️ 采信了调用方传的值 —— 客户端就能 1 分钱买 100 件）", o.TotalCents)
	}
}

// TestOrders_CreateRollsBackEverything 是事务这块最重要的一条。
//
// 第二个 item 违反唯一约束（同订单重复 SKU），整个 Create 必须回滚 ——
// orders 表里【一行都不能剩】。
//
// ⚠️ 只断言「返回了错误」是不够的：忘了 Rollback 的实现照样返回错误，
// 但会在库里留下一张没有 item 的孤儿订单。必须查表。
func TestOrders_CreateRollsBackEverything(t *testing.T) {
	repo, db := freshRepo(t)

	o := &orders.Order{
		Customer: "bob",
		Items: []orders.Item{
			{SKU: "DUP", Qty: 1, PriceCents: 100},
			{SKU: "DUP", Qty: 1, PriceCents: 100}, // ⚠️ 重复
		},
	}
	err := repo.Create(context.Background(), o)
	if err == nil {
		t.Fatal("重复 SKU 应该失败（migrations 里有 UNIQUE(order_id, sku)）")
	}

	if n := countRows(t, db, "orders"); n != 0 {
		t.Errorf("orders 表里剩了 %d 行, want 0\n"+
			"（⚠️ 事务没回滚 —— 留下了一张没有 item 的孤儿订单）", n)
	}
	if n := countRows(t, db, "order_items"); n != 0 {
		t.Errorf("order_items 表里剩了 %d 行, want 0", n)
	}
}

func TestOrders_CreateErrorKinds(t *testing.T) {
	tests := []struct {
		name  string
		order *orders.Order
		want  apperr.Kind
	}{
		{
			name: "重复 SKU → Conflict",
			order: &orders.Order{Customer: "c", Items: []orders.Item{
				{SKU: "X", Qty: 1, PriceCents: 100},
				{SKU: "X", Qty: 1, PriceCents: 100},
			}},
			want: apperr.KindConflict,
		},
		{
			name: "数量为 0 → Invalid",
			order: &orders.Order{Customer: "c", Items: []orders.Item{
				{SKU: "X", Qty: 0, PriceCents: 100},
			}},
			want: apperr.KindInvalid,
		},
		{
			name: "价格为负 → Invalid",
			order: &orders.Order{Customer: "c", Items: []orders.Item{
				{SKU: "X", Qty: 1, PriceCents: -1},
			}},
			want: apperr.KindInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := freshRepo(t)
			err := repo.Create(context.Background(), tt.order)
			if err == nil {
				t.Fatal("应该返回错误")
			}
			kind, ok := apperr.KindOf(err)
			if !ok {
				t.Fatalf("错误没被翻译成 apperr.Error: %v\n"+
					"（repository 的职责就是翻译 —— D12 §5）", err)
			}
			if kind != tt.want {
				t.Errorf("Kind = %v, want %v（err = %v）", kind, tt.want, err)
			}
		})
	}
}

// TestOrders_CreateDoesNotLeakDBDetails 锁住「内部细节不给用户」。
func TestOrders_CreateDoesNotLeakDBDetails(t *testing.T) {
	repo, _ := freshRepo(t)

	o := &orders.Order{Customer: "c", Items: []orders.Item{
		{SKU: "X", Qty: 1, PriceCents: 100},
		{SKU: "X", Qty: 1, PriceCents: 100},
	}}
	err := repo.Create(context.Background(), o)
	if err == nil {
		t.Fatal("应该返回错误")
	}

	_, msg := apperr.HTTPStatus(err)
	// 约束名、表名、SQLSTATE 都是内部结构信息，不该给客户端
	for _, leak := range []string{"order_items", "sku", "SQLSTATE", "constraint", "23505"} {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(leak)) {
			t.Errorf("用户可见消息里泄漏了 %q:\n  %s\n"+
				"（别把 pgErr.Message 直接当成 apperr 的 Message）", leak, msg)
		}
	}
	// 但完整细节必须留在 err 里，给日志用
	if !strings.Contains(err.Error(), "23505") && !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err.Error() 里应该保留原始错误细节，给日志排查用，得到:\n  %s", err.Error())
	}
}

// ---------- Get ----------

func TestOrders_GetRoundTrip(t *testing.T) {
	repo, _ := freshRepo(t)
	ctx := context.Background()

	o := sampleOrder()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, o.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Customer != "alice" || got.Note != "尽快发货" {
		t.Errorf("Customer=%q Note=%q", got.Customer, got.Note)
	}
	if got.Status != orders.StatusPending {
		t.Errorf("Status = %q, want %q（数据库默认值）", got.Status, orders.StatusPending)
	}
	if got.TotalCents != 2250 {
		t.Errorf("TotalCents = %d, want 2250", got.TotalCents)
	}
	if len(got.Items) != 2 {
		t.Fatalf("拿到 %d 个 item, want 2", len(got.Items))
	}
	// ⭐ 顺序要稳定，否则测试和 API 都会随机抖动
	if got.Items[0].SKU != "A-1" || got.Items[1].SKU != "B-2" {
		t.Errorf("item 顺序 = %q, %q；want A-1, B-2（按 id 升序）",
			got.Items[0].SKU, got.Items[1].SKU)
	}
	if got.Items[0].Qty != 2 || got.Items[0].PriceCents != 500 {
		t.Errorf("Items[0] = %+v", got.Items[0])
	}
}

// TestOrders_GetHandlesNullNote 考 NULL 处理（§6）。
//
// ⚠️ 这里【特意插了一条 item】—— 只插订单不插 item 的话，这条测试会
// 同时受「NULL 处理」和「订单没有 item 时能不能查到」两件事影响，
// 失败信息就会指错方向。后者由 TestOrders_GetOrderWithNoItems 单独测。
func TestOrders_GetHandlesNullNote(t *testing.T) {
	repo, db := freshRepo(t)
	ctx := context.Background()

	var id int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO orders (customer, note) VALUES ('nobody', NULL) RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("插入 NULL note: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO order_items (order_id, sku, qty, price_cents) VALUES ($1,'S',1,100)`,
		id); err != nil {
		t.Fatalf("插入 item: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get 撞上 NULL note 就失败了: %v\n"+
			"（⚠️ NULL 扫进 string 会报 converting NULL to string is unsupported —— 讲义 §6）", err)
	}
	if got.Note != "" {
		t.Errorf("Note = %q, want \"\"（NULL 映射成空字符串）", got.Note)
	}
}

// TestOrders_GetOrderWithNoItems 锁住「零个 item 的订单也要能查到」。
//
// ⭐ schema 里没有任何约束禁止「订单没有 item」——
// 直接 INSERT INTO orders 就能造出来，Create 传一个空的 Items 也会。
// 所以它是【合法状态】，Get 必须返回它（Items 为空），而不是 404。
//
// ⚠️ 这条专门抓 INNER JOIN：
//
//	FROM orders o INNER JOIN order_items oi ON ...   ← 零 item 的订单直接消失
//	FROM orders o LEFT  JOIN order_items oi ON ...   ← 保留，item 列全是 NULL
//
// 用 LEFT JOIN 的话别忘了：item 那几列会是 NULL，Scan 进 int64/string 会报错，
// 得先判断。这也是「一次 JOIN」vs「两次查询」要权衡的地方之一。
func TestOrders_GetOrderWithNoItems(t *testing.T) {
	repo, db := freshRepo(t)
	ctx := context.Background()

	var id int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO orders (customer) VALUES ('无 item 的订单') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("插入订单: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("查一张【没有 item】的订单失败了: %v\n"+
			"（⚠️ 用了 INNER JOIN 吗？零 item 的订单是合法状态，不该返回 404）", err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}
	if got.Customer != "无 item 的订单" {
		t.Errorf("Customer = %q", got.Customer)
	}
	if len(got.Items) != 0 {
		t.Errorf("Items 有 %d 个, want 0", len(got.Items))
	}
}

func TestOrders_GetNotFound(t *testing.T) {
	repo, _ := freshRepo(t)

	_, err := repo.Get(context.Background(), 999999)
	if err == nil {
		t.Fatal("查不存在的订单应该返回错误")
	}

	kind, ok := apperr.KindOf(err)
	if !ok || kind != apperr.KindNotFound {
		t.Errorf("Kind = %v (ok=%v), want KindNotFound\n"+
			"（sql.ErrNoRows 要在 repository 层翻译成 apperr.NotFound）", kind, ok)
	}

	// ⚠️ 这里【不】断言 errors.Is(err, sql.ErrNoRows)。
	//
	// 我最初是那么写的，但它有两个问题：
	//   ① 和 D12 §5 的分层原则矛盾 —— repository 的职责就是把存储细节翻译掉，
	//      上层该用 apperr.KindOf，不该依赖 sql.ErrNoRows（换个数据库就没这个哨兵了）
	//   ② 它强制了实现形状 —— 用 Query + JOIN 的实现压根不会产生 sql.ErrNoRows，
	//      那条断言等于要求「必须用 QueryRow」，而那不是这题要考的
	//
	// 真正要求的只有两条：Kind 对、消息对排查有用。

	status, _ := apperr.HTTPStatus(err)
	if status != 404 {
		t.Errorf("HTTPStatus = %d, want 404", status)
	}
}

// ---------- List ----------

func seed(t *testing.T, repo *orders.Repo, specs ...[2]string) {
	t.Helper()
	ctx := context.Background()
	for i, sp := range specs {
		o := &orders.Order{
			Customer: sp[0],
			Items:    []orders.Item{{SKU: fmt.Sprintf("S-%d", i), Qty: 1, PriceCents: 100}},
		}
		if err := repo.Create(ctx, o); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if sp[1] != "" && sp[1] != orders.StatusPending {
			if err := repo.UpdateStatus(ctx, o.ID, sp[1]); err != nil {
				t.Fatalf("seed UpdateStatus: %v", err)
			}
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestOrders_ListFilters(t *testing.T) {
	repo, _ := freshRepo(t)
	seed(t,
		repo,
		[2]string{"alice", orders.StatusPending},
		[2]string{"alice", orders.StatusPaid},
		[2]string{"bob", orders.StatusPaid},
	)

	tests := []struct {
		name   string
		filter orders.ListFilter
		want   int
	}{
		{"不过滤", orders.ListFilter{}, 3},
		{"按 customer", orders.ListFilter{Customer: ptr("alice")}, 2},
		{"按 status", orders.ListFilter{Status: ptr(orders.StatusPaid)}, 2},
		{"两个条件都用", orders.ListFilter{Customer: ptr("alice"), Status: ptr(orders.StatusPaid)}, 1},
		{"匹配不到", orders.ListFilter{Customer: ptr("nobody")}, 0},
		{"空字符串是【有效过滤值】不是不过滤", orders.ListFilter{Customer: ptr("")}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.List(context.Background(), tt.filter)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("拿到 %d 条, want %d", len(got), tt.want)
			}
		})
	}
}

// TestOrders_ListEmptyIsNotAnError 锁住「查不到不是错误」。
func TestOrders_ListEmptyIsNotAnError(t *testing.T) {
	repo, _ := freshRepo(t)

	got, err := repo.List(context.Background(), orders.ListFilter{})
	if err != nil {
		t.Fatalf("空表 List 不该报错: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("拿到 %d 条, want 0", len(got))
	}
	// ⭐ 返回空切片而不是 nil：JSON 序列化时 nil 是 null，空切片才是 []
	// （D12 §1.4 那条 —— 前端最烦这个）
	if got == nil {
		t.Error("List 查不到时返回了 nil，应该返回空切片\n" +
			"（nil 序列化成 null，[]Order{} 才序列化成 []）")
	}
}

func TestOrders_ListLimit(t *testing.T) {
	repo, db := freshRepo(t)

	// ⭐ 批量插 MaxLimit+50 行 —— 只插 5 行的话，
	// 「截断到 MaxLimit」和「压根不截断」结果一样，测不出区别。
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO orders (customer, status)
		 SELECT 'alice', 'pending' FROM generate_series(1, $1)`,
		orders.MaxLimit+50); err != nil {
		t.Fatalf("批量插入: %v", err)
	}
	total := orders.MaxLimit + 50

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"limit=2", 2, 2},
		{"limit=0 用默认值", 0, orders.DefaultLimit},
		{"limit 为负也用默认值", -1, orders.DefaultLimit},
		{"limit 超过 MaxLimit 要截断到 MaxLimit", total + 1000, orders.MaxLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.List(context.Background(), orders.ListFilter{Limit: tt.limit})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("拿到 %d 条, want %d", len(got), tt.want)
			}
		})
	}
}

func TestOrders_ListOrderedByIDDesc(t *testing.T) {
	repo, _ := freshRepo(t)
	seed(t, repo,
		[2]string{"a", ""}, [2]string{"b", ""}, [2]string{"c", ""})

	got, err := repo.List(context.Background(), orders.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].ID <= got[i].ID {
			t.Fatalf("结果不是按 id 降序: %v", []int64{got[i-1].ID, got[i].ID})
		}
	}
}

// TestOrders_ListSQLInjection 锁住「参数走占位符」。
//
// ⚠️ 如果实现里用 fmt.Sprintf 把 customer 拼进 WHERE，下面任何一条都会
// 造成语法错误、返回全部行、或者真的把表删了。
func TestOrders_ListSQLInjection(t *testing.T) {
	repo, db := freshRepo(t)
	seed(t, repo, [2]string{"alice", ""}, [2]string{"bob", ""})

	payloads := []string{
		`alice' OR '1'='1`,
		`'; DROP TABLE order_items; --`,
		`alice"`,
		`alice\`,
		`%`,   // LIKE 的通配符 —— 如果实现用了 LIKE 就会全匹配
		`_`,   //
		`\''`, //
	}

	for _, p := range payloads {
		t.Run(p, func(t *testing.T) {
			got, err := repo.List(context.Background(), orders.ListFilter{Customer: ptr(p)})
			if err != nil {
				t.Fatalf("SQL 注入载荷让查询报错了（说明被拼进 SQL 了）: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("载荷 %q 匹配到了 %d 条 —— 应该是 0 条\n"+
					"（参数必须走 $1 占位符，不能拼字符串，也不要用 LIKE）", p, len(got))
			}
		})
	}

	// 表还在吗
	if n := countRows(t, db, "orders"); n != 2 {
		t.Errorf("跑完注入测试之后 orders 表里有 %d 行, want 2", n)
	}
}

// ---------- UpdateStatus ----------

func TestOrders_UpdateStatus(t *testing.T) {
	repo, _ := freshRepo(t)
	ctx := context.Background()

	o := sampleOrder()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.UpdateStatus(ctx, o.ID, orders.StatusPaid); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err := repo.Get(ctx, o.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != orders.StatusPaid {
		t.Errorf("Status = %q, want %q", got.Status, orders.StatusPaid)
	}
}

// TestOrders_UpdateStatusNotFound 考 RowsAffected。
//
// ⚠️ UPDATE 一个不存在的 id 【不会报错】，只是影响 0 行。
// 不检查的话，「改一个不存在的订单」会静默成功。
func TestOrders_UpdateStatusNotFound(t *testing.T) {
	repo, _ := freshRepo(t)

	err := repo.UpdateStatus(context.Background(), 999999, orders.StatusPaid)
	if err == nil {
		t.Fatal("改一个不存在的订单应该报错\n" +
			"（⚠️ UPDATE 不存在的行不会报错，只是 RowsAffected()==0 —— 要自己检查）")
	}
	if kind, ok := apperr.KindOf(err); !ok || kind != apperr.KindNotFound {
		t.Errorf("Kind = %v (ok=%v), want KindNotFound", kind, ok)
	}
}

func TestOrders_UpdateStatusRejectsInvalid(t *testing.T) {
	repo, _ := freshRepo(t)
	ctx := context.Background()

	o := sampleOrder()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, bad := range []string{"", "PAID", "deleted", "pending "} {
		t.Run(fmt.Sprintf("%q", bad), func(t *testing.T) {
			err := repo.UpdateStatus(ctx, o.ID, bad)
			if err == nil {
				t.Fatalf("状态 %q 应该被拒绝", bad)
			}
			if kind, ok := apperr.KindOf(err); !ok || kind != apperr.KindInvalid {
				t.Errorf("Kind = %v (ok=%v), want KindInvalid\n"+
					"（在 Go 这边先挡一道，别指望数据库的 CHECK —— "+
					"那样的错误信息对用户没有意义）", kind, ok)
			}
		})
	}
}

// ---------- ⭐ 今天的题眼：连接泄漏 ----------

// withNullable 临时去掉某列的 NOT NULL，让我们能插入一行【坏数据】。
// 测试结束自动恢复。
func withNullable(t *testing.T, db *sql.DB, table, col string) {
	t.Helper()
	ctx := context.Background()
	//nolint:gosec // table/col 都是测试里写死的常量
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL`, table, col)); err != nil {
		t.Fatalf("放开 NOT NULL: %v", err)
	}
	t.Cleanup(func() {
		//nolint:gosec // 同上
		db.ExecContext(context.Background(), //nolint:errcheck
			fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s SET NOT NULL`, table, col))
	})
}

// TestOrders_NoConnectionLeakOnScanError 是今天最重要的一条。
//
// # 为什么要专门制造 Scan 失败
//
// 讲义 §4.1：rows 读到底会【自动关闭】。所以 happy path 上就算没写
// defer rows.Close()，连接也会正常归还 —— 测试全绿，你以为没问题。
//
// 真正泄漏的是【提前退出】：循环里 Scan 出错 return、或者 break。
// 这条测试就是去踩那条路径：往表里插一行 NULL customer，让 Scan 报
// 「converting NULL to string is unsupported」，逼实现从循环中间返回。
//
// 判据：调用返回之后，InUse 必须回到 0。漏了 Close 的话它会一直是 1。
//
// ⚠️ 这也说明「坏数据不该拖垮服务」：一行脏数据只该让一个请求失败，
// 而不该永久吃掉一条数据库连接。
func TestOrders_NoConnectionLeakOnScanError(t *testing.T) {
	db := openDB(t)
	truncate(t, db)
	ctx := context.Background()

	// 独立的小池子，泄漏立刻可见
	small, err := sql.Open("pgx", dsn())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer small.Close() //nolint:errcheck
	small.SetMaxOpenConns(2)
	small.SetMaxIdleConns(2)
	repo := orders.NewRepo(small)

	t.Run("List 的迭代", func(t *testing.T) {
		withNullable(t, db, "orders", "customer")
		if _, err := db.ExecContext(ctx,
			`INSERT INTO orders (customer) VALUES (NULL)`); err != nil {
			t.Fatalf("插入坏数据: %v", err)
		}
		t.Cleanup(func() {
			db.ExecContext(ctx, `DELETE FROM orders WHERE customer IS NULL`) //nolint:errcheck
		})

		for i := 1; i <= 4; i++ {
			c, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, err := repo.List(c, orders.ListFilter{})
			cancel()

			if err == nil {
				t.Fatal("撞上 NULL customer，Scan 应该失败")
			}
			if st := small.Stats(); st.InUse != 0 {
				t.Fatalf("第 %d 次 List 出错返回后，还有 %d 条连接在使用中\n"+
					"⚠️ 循环中间 return 时没关 rows —— 少了 defer rows.Close()\n"+
					"池状态: %+v", i, st.InUse, st)
			}
		}
	})

	t.Run("Get 的 items 迭代", func(t *testing.T) {
		truncate(t, db)

		o := sampleOrder()
		if err := repo.Create(ctx, o); err != nil {
			t.Fatalf("Create: %v", err)
		}
		withNullable(t, db, "order_items", "sku")
		if _, err := db.ExecContext(ctx,
			`INSERT INTO order_items (order_id, sku, qty, price_cents) VALUES ($1, NULL, 1, 1)`,
			o.ID); err != nil {
			t.Fatalf("插入坏数据: %v", err)
		}

		for i := 1; i <= 4; i++ {
			c, cancel := context.WithTimeout(ctx, 3*time.Second)
			_, err := repo.Get(c, o.ID)
			cancel()

			if err == nil {
				t.Fatal("撞上 NULL sku，Scan 应该失败")
			}
			if st := small.Stats(); st.InUse != 0 {
				t.Fatalf("第 %d 次 Get 出错返回后，还有 %d 条连接在使用中\n"+
					"⚠️ 少了 defer rows.Close()\n池状态: %+v", i, st.InUse, st)
			}
		}
	})
}

// ⚠️ rows.Err() 这一条【没有】自动化测试，只能靠 review —— 说明一下原因。
//
// 要让 rows.Err() 和「不查 rows.Err()」产生可观测的差异，必须制造
// 「迭代中途才失败」：前几批数据成功返回，后面才出错。讲义 §5 用的是
//
//	SELECT 1/(50000-i) FROM generate_series(1,60000) i
//
// ——但那条 SQL 在 List 内部，测试注入不进去。而 ctx 取消这条路也不行：
// List 有 LIMIT，500 行一批就取完了，根本没有「中途」。
//
// 结论：这是「有要求但无可观测行为差异」的情况（和 D12 那个 slog.LogValuer
// 一样）。与其编造一个脆弱的测试，不如写清楚 —— review 时我逐行看这两处：
//
//	Get  里的 items 循环
//	List 里的 orders 循环
//
// 都必须是「defer rows.Close() / 循环 / rows.Err()」三件套。

// TestOrders_NoConnectionLeakUnderLoad 是上一条的补充：正常路径跑很多遍，
// 池也不能被吃掉（抓「事务忘了结束」「QueryRow 之后忘了处理」这类）。
func TestOrders_NoConnectionLeakUnderLoad(t *testing.T) {
	db := openDB(t)
	truncate(t, db)

	small, err := sql.Open("pgx", dsn())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer small.Close() //nolint:errcheck
	small.SetMaxOpenConns(2)
	small.SetMaxIdleConns(2)
	repo := orders.NewRepo(small)
	ctx := context.Background()

	o := sampleOrder()
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	call := func(name string, fn func(context.Context) error) {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		start := time.Now()
		err := fn(c)
		el := time.Since(start)

		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s 卡住了（%v）—— 连接池被耗干\n池状态: %+v", name, el, small.Stats())
		}
		if el > time.Second {
			t.Errorf("%s 花了 %v —— 在排队等连接，说明有泄漏", name, el)
		}
	}

	for i := range 8 {
		call(fmt.Sprintf("List#%d", i), func(c context.Context) error {
			_, err := repo.List(c, orders.ListFilter{})
			return err
		})
		call(fmt.Sprintf("Get#%d", i), func(c context.Context) error {
			_, err := repo.Get(c, o.ID)
			return err
		})
		call(fmt.Sprintf("Get(不存在)#%d", i), func(c context.Context) error {
			_, _ = repo.Get(c, 999999) // 预期失败，不算测试失败
			return nil
		})
		call(fmt.Sprintf("List(空结果)#%d", i), func(c context.Context) error {
			_, err := repo.List(c, orders.ListFilter{Customer: ptr("nobody")})
			return err
		})
	}

	if st := small.Stats(); st.InUse != 0 {
		t.Errorf("跑完之后还有 %d 条连接在使用中, want 0\n池状态: %+v", st.InUse, st)
	}
}

// TestOrders_CreateLeaksNothingOnFailure 单独测事务失败路径的连接归还。
//
// 事务独占一条连接。失败时如果既没 Commit 也没 Rollback，那条连接
// 永远回不了池子 —— 比 rows 泄漏更致命，因为事务还会一直持有数据库端的锁。
func TestOrders_CreateLeaksNothingOnFailure(t *testing.T) {
	db := openDB(t)
	truncate(t, db)

	txDB, err := sql.Open("pgx", dsn())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer txDB.Close() //nolint:errcheck
	txDB.SetMaxOpenConns(2)
	txDB.SetMaxIdleConns(2)

	repo := orders.NewRepo(txDB)

	for i := range 6 {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		o := &orders.Order{Customer: "c", Items: []orders.Item{
			{SKU: "DUP", Qty: 1, PriceCents: 100},
			{SKU: "DUP", Qty: 1, PriceCents: 100}, // 必然失败
		}}
		start := time.Now()
		err := repo.Create(ctx, o)
		el := time.Since(start)
		cancel()

		if err == nil {
			t.Fatal("重复 SKU 应该失败")
		}
		if el > time.Second {
			t.Fatalf("第 %d 次 Create 花了 %v —— 事务连接没还回池子\n"+
				"（⚠️ 失败路径上既没 Commit 也没 Rollback）池状态: %+v", i, el, txDB.Stats())
		}
	}

	if st := txDB.Stats(); st.InUse != 0 {
		t.Errorf("跑完之后还有 %d 条连接在使用中, want 0\n池状态: %+v", st.InUse, st)
	}
}
