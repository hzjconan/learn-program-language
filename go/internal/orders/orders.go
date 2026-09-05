// Package orders 是订单的 repository 层（D13）。
//
// 分层职责（D12 §5）：
//
//	repository（本包）  把 SQL / 驱动的错误【翻译】成 apperr.Error
//	service            用 %w 加上下文，不改 Kind
//	handler            调 apperr.HTTPStatus，映射成状态码
//
// ⚠️ 本包【不 import net/http】—— 它不知道自己会被 HTTP 还是 gRPC 调用。
package orders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
)

// Order 是一张订单。
type Order struct {
	ID       int64
	Customer string
	// Note 是可选备注。数据库里这一列可以为 NULL —— 你决定怎么表达「没有备注」，
	// 并在 NOTES 里写下理由（讲义 §6 给了四种选择）。
	Note       string
	Status     string
	TotalCents int64
	CreatedAt  time.Time
	Items      []Item
}

// Item 是订单里的一行商品。
type Item struct {
	ID         int64
	SKU        string
	Qty        int
	PriceCents int64
}

// 合法的订单状态。和 migrations/001_orders.sql 里的 CHECK 约束保持一致。
const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusShipped   = "shipped"
	StatusCancelled = "cancelled"
)

// Repo 是订单仓储。
//
// ⭐ 它持有 *sql.DB（连接池），不是连接。整个进程共用一个，并发安全（§1）。
type Repo struct {
	db *sql.DB
}

// NewRepo 构造一个 Repo。
func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// ListFilter 是 List 的过滤条件。
//
// ⭐ 用指针表达「不过滤」—— 和 D12 §1.2 的 PATCH 是同一个模式：
// nil 表示「这一维不过滤」，非 nil 表示「按这个值过滤（哪怕它是零值）」。
type ListFilter struct {
	Customer *string
	Status   *string
	// Limit 为 0 时用 DefaultLimit。
	Limit int
}

// DefaultLimit 是 List 不指定 Limit 时的默认条数。
//
// ⚠️ 列表接口【必须】有上限。没有 LIMIT 的查询在表长大之后会
// 一次性把几百万行拉进内存 —— 这是另一种「整个服务挂住」。
const DefaultLimit = 50

// MaxLimit 是 List 允许的最大条数。
const MaxLimit = 500

// Create 在一个事务里插入订单和它的所有 item。
//
// TODO(D13)：实现我。
//
// 要求：
//
//   - 用 BeginTx 开事务，任何一步失败都要【整体回滚】，
//     且成功路径不能因为 defer 里的 Rollback 而报错（§7）
//   - 插 orders 用 `RETURNING id, created_at` 一次拿回自增主键和时间戳，
//     写进 o.ID / o.CreatedAt
//   - 逐条插 order_items，把生成的 id 写回 o.Items[i].ID
//   - TotalCents 由服务端算：sum(qty * price_cents)，【不要】信调用方传的值
//
// 错误翻译（§8，用 errors.As 拿 *pgconn.PgError 看 Code）：
//
//	23505 unique_violation      → apperr.Conflict（同一订单里重复的 SKU）
//	23514 check_violation       → apperr.Invalid（qty <= 0、金额为负、status 非法）
//	其他                         → apperr.Internal
//
// ⚠️ 三个容易踩的：
//
//  1. 事务里必须全用 tx.，一个 r.db. 都不能有 —— 那条语句不在事务里，回滚撤不掉
//  2. 返回值要写成命名的 (err error)，否则 defer 改不了它（D2 §5）
//  3. 别把 pgErr.Message 直接放进给用户的消息里，它含表名/约束名
func (r *Repo) Create(ctx context.Context, o *Order) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if rbe := tx.Rollback(); rbe != nil && !errors.Is(rbe, sql.ErrTxDone) {
			err = errors.Join(err, rbe)
		}
	}()

	insertOrderSQL := "INSERT INTO orders (customer, note, status, total_cents) VALUES ($1, $2, COALESCE($3, 'pending'), $4) RETURNING id, created_at"

	o.TotalCents = 0
	for _, v := range o.Items {
		o.TotalCents += int64(v.Qty) * v.PriceCents
	}

	// 把 note 转换为指针， 如果note本身是空string，插入数据库后的值应该是null
	var note *string
	if o.Note != "" {
		note = &o.Note
	}

	// 把 status 转换为指针， 如果status本身是空string，利用COALESCE函数来转换成默认值pending
	var status *string
	if o.Status != "" {
		status = &o.Status
	}

	if oerr := tx.QueryRowContext(ctx, insertOrderSQL, o.Customer, note, status, o.TotalCents).Scan(&o.ID, &o.CreatedAt); oerr != nil {
		var pge *pgconn.PgError
		if errors.As(oerr, &pge) && pge.Code == "23514" {
			return apperr.Invalid("订单参数不合法", oerr)
		}
		return apperr.Internal("插入订单失败", oerr)
	}

	insertItemSQL := "INSERT INTO order_items (order_id, sku, qty, price_cents) VALUES ($1, $2, $3, $4) RETURNING id"

	for i := range o.Items {
		if ierr := tx.QueryRowContext(ctx, insertItemSQL, o.ID, &o.Items[i].SKU, &o.Items[i].Qty, &o.Items[i].PriceCents).Scan(&o.Items[i].ID); ierr != nil {
			var pge *pgconn.PgError
			if errors.As(ierr, &pge) {
				if pge.Code == "23505" {
					return apperr.Conflict("订单物品重复", ierr)
				}
				if pge.Code == "23514" {
					return apperr.Invalid("订单物品数量或价格不合法", ierr)
				}
			}
			return apperr.Internal("插入订单明细失败", ierr)
		}
	}

	return tx.Commit()
}

// Get 按 id 查订单，连同它的所有 item。
//
// TODO(D13)：实现我。
//
//   - 订单不存在 → apperr.NotFound，消息里带上 id（方便排查）
//   - ⭐【零个 item 的订单是合法状态】—— schema 没禁止它，Create 传空 Items 也会造出来。
//     Get 必须返回它（Items 为空），不能返回 404。用 INNER JOIN 会踩这个坑。
//   - Items 按 id 升序返回，保证结果稳定
//   - note 列可能是 NULL —— 处理它（§6）
//
// ⚠️ 查 items 要用 Query + 迭代，正是 §4/§5 那三行套路的考点：
// defer rows.Close() / 循环 / rows.Err()，一个都不能少。
func (r *Repo) Get(ctx context.Context, id int64) (*Order, error) {
	o := &Order{}

	orderQ := "select id, customer, COALESCE(note, ''), status, total_cents, created_at from orders where id = $1"
	err := r.db.QueryRowContext(ctx, orderQ, id).Scan(&o.ID, &o.Customer, &o.Note, &o.Status, &o.TotalCents, &o.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound(fmt.Sprintf("订单号 %d 不存在", id), nil)
		}
		return nil, apperr.Internal("查询订单失败", err)
	}

	itemQ := "select id, sku, qty, price_cents from order_items where order_id = $1 order by id"

	rows, err := r.db.QueryContext(ctx, itemQ, id)

	if err != nil {
		return nil, apperr.Internal("查询订单明细失败", err)
	}

	defer rows.Close() //nolint: errcheck

	for rows.Next() {
		item := Item{}
		if err := rows.Scan(&item.ID, &item.SKU, &item.Qty, &item.PriceCents); err != nil {
			return nil, apperr.Internal("查询订单明细失败", err)
		}
		o.Items = append(o.Items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, apperr.Internal("查询订单明细失败", err)
	}

	return o, nil
}

// List 按条件列出订单（不含 Items）。
//
// TODO(D13)：实现我。
//
//   - f.Customer / f.Status 为 nil 表示该维度不过滤
//   - f.Limit <= 0 用 DefaultLimit；超过 MaxLimit 截断到 MaxLimit
//   - 按 id 降序（最新的在前）
//   - 查不到任何订单【不是错误】，返回空切片和 nil
//
// ⚠️ 条件是动态的，但【绝对不允许】用 fmt.Sprintf 拼 SQL（§3）。
// 提示：把条件和参数分别 append 到两个切片里，最后用 strings.Join 拼
// WHERE 子句，参数始终走占位符。占位符编号要和参数顺序对上。
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Order, error) {
	orders := []Order{}

	var q strings.Builder
	q.WriteString("select id, customer, COALESCE(note, ''), status, total_cents, created_at from orders where 1=1")

	var params []any

	if f.Customer != nil {
		params = append(params, *f.Customer)
		fmt.Fprintf(&q, " and customer = $%d", len(params))
	}

	if f.Status != nil {
		params = append(params, *f.Status)
		fmt.Fprintf(&q, " and status = $%d", len(params))
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	params = append(params, limit)
	fmt.Fprintf(&q, " order by id desc limit $%d", len(params))

	rows, err := r.db.QueryContext(ctx, q.String(), params...)

	if err != nil {
		return nil, apperr.Internal("查询订单失败", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		order := Order{}
		if err := rows.Scan(&order.ID, &order.Customer, &order.Note, &order.Status, &order.TotalCents, &order.CreatedAt); err != nil {
			return nil, apperr.Internal("查询订单失败", err)
		}
		orders = append(orders, order)
	}

	if rows.Err() != nil {
		return nil, apperr.Internal("查询订单失败", rows.Err())
	}

	return orders, nil
}

// UpdateStatus 修改订单状态。
//
// TODO(D13)：实现我。
//
//   - status 不在四个合法值里 → apperr.Invalid（在 Go 这边先挡一道，
//     别指望数据库的 CHECK —— 那样错误信息对用户没有意义）
//   - 订单不存在 → apperr.NotFound
//
// ⭐ 这题的考点是 RowsAffected()：UPDATE 一个不存在的 id 【不会报错】，
// 它只是影响了 0 行。不检查的话，「改一个不存在的订单」会静默成功。
func (r *Repo) UpdateStatus(ctx context.Context, id int64, status string) error {

	if status != StatusPending && status != StatusPaid && status != StatusShipped && status != StatusCancelled {
		return apperr.Invalid("订单状态不合法", nil)
	}

	result, err := r.db.ExecContext(ctx, "update orders set status = $1 where id = $2", status, id)
	if err != nil {
		return apperr.Internal("更新订单状态失败", err)
	}
	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 { //nolint: errcheck
		return apperr.NotFound(fmt.Sprintf("订单号 %d 不存在", id), nil)
	}
	return nil
}
