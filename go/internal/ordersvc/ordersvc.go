// Package ordersvc 是订单的业务逻辑层（D14）。
//
// # 它负责什么
//
//	业务规则     「订单至少要有一个商品」「pending 才能支付」
//	编排         需要多个 repository 协作时，在这里组织
//	事务边界     跨多个存储操作的一致性（本课还用不到，D13 的 Create 自带事务）
//
// # 它【不】负责什么
//
//	⚠️ 不碰 http.Request / http.ResponseWriter —— 它不知道自己被 HTTP 还是 gRPC 调用
//	⚠️ 不碰 SQL / sql.Rows —— 那是 repository 的事
//
// # ⭐ 依赖方向（D14 §2，今天的题眼）
//
// 本包定义它【需要】的接口 Repository，而不是 import 某个具体实现。
// orders.Repo 恰好满足这个接口，但它自己【不知道】—— 这是 Go 的隐式实现（D5 §1）。
//
// 组装发生在 cmd/api/main.go，只有那里同时知道两边。
package ordersvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
	"github.com/hzjconan/learn-program-language/go/internal/orders"
)

// MaxItemsPerOrder 是一张订单最多能有多少种商品。
//
// ⚠️ 这是【业务规则】，不是存储限制 —— 所以它在 service 层，不在 repository。
const MaxItemsPerOrder = 100

// MaxQtyPerItem 是单个商品的数量上限。
const MaxQtyPerItem = 1000

// Repository 是 ordersvc 需要的存储能力。
//
// ⭐ 定义在【消费方】。注意本包没有 import 任何数据库相关的东西 ——
// 实测：接口定义在消费方时依赖闭包 42 个包，直接依赖具体实现是 205 个（§2.1）。
//
// ⚠️ 只列你真正用到的方法。orders.Repo 上有什么不重要，
// 重要的是【本包用了什么】（§2.4 小接口哲学）。
type Repository interface {
	Create(ctx context.Context, o *orders.Order) error
	Get(ctx context.Context, id int64) (*orders.Order, error)
	List(ctx context.Context, f orders.ListFilter) ([]orders.Order, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
}

// Service 是订单业务逻辑。
type Service struct {
	repo Repository
}

// New 构造 Service。
//
// ⭐ 这就是「依赖注入」的全部 —— 一个构造函数，一个接口参数。
// 不需要容器、不需要注解、不需要反射。出错是编译错误，不是启动时 panic。
func New(repo Repository) *Service { return &Service{repo: repo} }

// Place 下单。
//
// TODO(D14)：实现我。
//
// 业务校验（都返回 apperr.Invalid，消息要说清哪里不对）：
//
//   - Customer 不能为空（去掉首尾空白之后）
//   - Items 不能为空 —— ⭐ 这条最重要：空订单在 repository 层是【合法】的
//     （D13 那个 TestOrders_GetOrderWithNoItems 就是证明），
//     所以必须由业务层来禁止
//   - Items 数量不能超过 MaxItemsPerOrder
//   - 每个 item：SKU 非空、Qty 在 1..MaxQtyPerItem、PriceCents >= 0
//   - 同一张订单里 SKU 不能重复 —— ⚠️ 数据库有唯一约束会拦，但那样返回的是
//     Conflict（409）。业务层先挡一道，返回 Invalid（400）语义更准：
//     这是【请求本身有问题】，不是「和现有数据冲突」
//
// 校验通过后：
//
//  1. ⭐ 算总额 —— `sum(Qty × PriceCents)`
//  2. 调 repo.Create
//
// # ⭐ 金额算在哪一层
//
// 建议给 orders.Order 加一个方法，然后在这里调它：
//
//	// internal/orders/orders.go
//	// ComputeTotal 按 Items 重算 TotalCents。
//	func (o *Order) ComputeTotal() {
//		var t int64
//		for _, it := range o.Items {
//			t += int64(it.Qty) * it.PriceCents
//		}
//		o.TotalCents = t
//	}
//
//	// 这里
//	o.ComputeTotal()
//	return s.repo.Create(ctx, o)
//
// 三层各管一段，正好是 §3 那条判据：
//
//	怎么算       Order 类型自己 —— 这是它的【不变量】，和派生它的数据待在一起
//	什么时候算   service —— 「下单时按当前明细结算」是业务决策
//	存下来       repository —— 它只负责持久化收到的东西
//
// ⚠️ 别让 repository 算。将来加优惠券时，Place 会变成
// `o.ComputeTotal(); o.ApplyCoupon(c)` —— 业务编排明明白白在 service 里。
// 如果金额是 repo 算的，你就得让 repo 知道优惠券，那就彻底歪了。
//
// ⚠️ 另外注意：「客户端不能自己定金额」这个【安全】边界不在这里，
// 也不在 repository —— 它在 api 层的 DTO（§4）：createOrderRequest 里
// 压根没有 total_cents 字段，客户端连传都传不进来。
// 本层算总额是为了【领域一致性】，不是为了防客户端。
//
// ⚠️ 别在这里检查「Status 是不是 pending」：新订单的状态由数据库默认值决定，
// 调用方压根不该传。
func (s *Service) Place(ctx context.Context, o *orders.Order) error {
	if strings.TrimSpace(o.Customer) == "" {
		return apperr.Invalid("Customer信息为空", nil)
	}
	if len(o.Items) == 0 {
		return apperr.Invalid("Items信息为空", nil)
	}
	if len(o.Items) > MaxItemsPerOrder {
		return apperr.Invalid(fmt.Sprintf("Items数量 %d 超过最大值 %d", len(o.Items), MaxItemsPerOrder), nil)
	}
	seen := make(map[string]struct{}, len(o.Items))
	for _, it := range o.Items {
		if it.SKU == "" {
			return apperr.Invalid("SKU信息为空", nil)
		}
		if _, dup := seen[it.SKU]; dup {
			return apperr.Invalid(fmt.Sprintf("SKU %s 重复", it.SKU), nil)
		}
		seen[it.SKU] = struct{}{}
		if it.Qty < 1 {
			return apperr.Invalid(fmt.Sprintf("Qty 必须大于 0，得到 %d", it.Qty), nil)
		}
		if it.Qty > MaxQtyPerItem {
			return apperr.Invalid(fmt.Sprintf("Qty %d 超过上限 %d", it.Qty, MaxQtyPerItem), nil)
		}
		if it.PriceCents < 0 {
			return apperr.Invalid(fmt.Sprintf("PriceCents %d 小于 0", it.PriceCents), nil)
		}
	}

	o.ComputeTotal()
	return s.repo.Create(ctx, o)
}

// Get 查订单。
//
// TODO(D14)：实现我。
//
//   - id <= 0 → apperr.Invalid（别把明显非法的 id 送到数据库去）
//   - 其余直接转发给 repo
//
// ⚠️ 这个方法只有三行是【正常的】—— 不是每个 service 方法都必须有复杂逻辑。
// 但如果你【所有】方法都只有一行转发，那这一层就是噪音（§3）。
func (s *Service) Get(ctx context.Context, id int64) (*orders.Order, error) {
	if id <= 0 {
		return nil, apperr.Invalid(fmt.Sprintf("id %d 不能小于等于 0", id), nil)
	}
	return s.repo.Get(ctx, id)
}

// List 列出订单。
//
// TODO(D14)：实现我。
//
//   - Status 非 nil 时，必须是四个合法状态之一，否则 apperr.Invalid
//     （⭐ 用户传 status=deleted 应该得到 400，而不是一个空列表 ——
//     空列表会让人以为「确实没有这种订单」）
//   - 其余转发给 repo（limit 的规范化由 repository 负责，D13 §11）
func (s *Service) List(ctx context.Context, f orders.ListFilter) ([]orders.Order, error) {
	if f.Status != nil && !f.IsStatusValid(*f.Status) {
		return nil, apperr.Invalid(fmt.Sprintf("Status %s 无效", *f.Status), nil)
	}
	return s.repo.List(ctx, f)
}

// Pay 支付订单。
//
// TODO(D14)：实现我。见 canTransition 的说明。
func (s *Service) Pay(ctx context.Context, id int64) error {
	o, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !canTransition(o.Status, orders.StatusPaid) {
		return apperr.Conflict(fmt.Sprintf("状态转换不允许 %s 到 %s", o.Status, orders.StatusPaid), nil)
	}
	return s.repo.UpdateStatus(ctx, id, orders.StatusPaid)
}

// Cancel 取消订单。
//
// TODO(D14)：实现我。见 canTransition 的说明。
func (s *Service) Cancel(ctx context.Context, id int64) error {
	o, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !canTransition(o.Status, orders.StatusCancelled) {
		return apperr.Conflict(fmt.Sprintf("状态转换不允许 %s 到 %s", o.Status, orders.StatusCancelled), nil)
	}
	return s.repo.UpdateStatus(ctx, id, orders.StatusCancelled)
}

// canTransition 判断状态转换是否合法。
//
// TODO(D14)：实现我（Pay / Cancel 都用它）。
//
// ⭐ 状态机是最典型的业务逻辑 —— 它必须在 service 层。
// 放 repository 的话，UpdateStatus 就变成了「知道业务含义的 SQL 函数」，
// 换个业务场景就没法复用（§3 判断题③）。
//
//	当前状态      pay          cancel
//	---------    ---------    ---------
//	pending      → paid       → cancelled
//	paid         ❌ 已支付     → cancelled
//	shipped      ❌           ❌ 已发货不能取消
//	cancelled    ❌           ❌ 已取消
//
// 非法转换返回 apperr.Conflict，⚠️ 消息里要说清【从什么状态到什么状态】——
// 只说「操作不允许」的话，用户和排查的人都不知道发生了什么。
//
// ⚠️ 「订单不存在」不该由这个函数处理 —— 那是 repo.Get 返回的 NotFound，
// 直接往上传（Pay/Cancel 里先 Get 再判断）。
func canTransition(from, to string) bool {
	switch from {
	case orders.StatusPending:
		return to == orders.StatusPaid || to == orders.StatusCancelled
	case orders.StatusPaid:
		return to == orders.StatusCancelled
	}
	// shipped / cancelled 以及未知状态：任何转换都不合法
	return false
}

// 骨架期占位：Pay / Cancel 实现完并用上 canTransition 之后，删掉这一行。
var _ = canTransition
