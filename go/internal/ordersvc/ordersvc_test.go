package ordersvc_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
	"github.com/hzjconan/learn-program-language/go/internal/orders"
	"github.com/hzjconan/learn-program-language/go/internal/ordersvc"
)

// ---------- 假 repository ----------
//
// ⭐ 就是个 struct，没有 mock 框架（§6）。
//
// 它是【普通代码】：能打断点、编译器会检查签名、改接口时这里会报错。
// mock 框架的「验证调用次数/参数」往往在测实现细节，不是行为。
//
// ⚠️ 注意本文件【没有】import database/sql 或任何驱动 ——
// 整个 service 的测试跑起来不需要 Postgres。这就是「接口定义在消费方」换来的。
type fakeRepo struct {
	byID   map[int64]*orders.Order
	nextID int64

	// 想让某个方法失败时设这些
	createErr error
	getErr    error
	listErr   error
	updateErr error

	// 记录调用，供断言用
	created      []*orders.Order
	updatedTo    []string
	updateCalled int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[int64]*orders.Order{}, nextID: 1}
}

func (f *fakeRepo) Create(_ context.Context, o *orders.Order) error {
	if f.createErr != nil {
		return f.createErr
	}
	o.ID = f.nextID
	f.nextID++
	o.Status = orders.StatusPending
	for _, it := range o.Items {
		o.TotalCents += int64(it.Qty) * it.PriceCents
	}
	cp := *o
	f.byID[o.ID] = &cp
	f.created = append(f.created, &cp)
	return nil
}

func (f *fakeRepo) Get(_ context.Context, id int64) (*orders.Order, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	o, ok := f.byID[id]
	if !ok {
		return nil, apperr.NotFound("订单不存在", nil)
	}
	cp := *o
	return &cp, nil
}

func (f *fakeRepo) List(_ context.Context, _ orders.ListFilter) ([]orders.Order, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := []orders.Order{}
	for _, o := range f.byID {
		out = append(out, *o)
	}
	return out, nil
}

func (f *fakeRepo) UpdateStatus(_ context.Context, id int64, status string) error {
	f.updateCalled++
	f.updatedTo = append(f.updatedTo, status)
	if f.updateErr != nil {
		return f.updateErr
	}
	o, ok := f.byID[id]
	if !ok {
		return apperr.NotFound("订单不存在", nil)
	}
	o.Status = status
	return nil
}

// seed 往假仓库里放一张指定状态的订单，返回它的 id。
func (f *fakeRepo) seed(status string) int64 {
	id := f.nextID
	f.nextID++
	f.byID[id] = &orders.Order{
		ID: id, Customer: "alice", Status: status,
		Items: []orders.Item{{ID: 1, SKU: "A", Qty: 1, PriceCents: 100}},
	}
	return id
}

func validOrder() *orders.Order {
	return &orders.Order{
		Customer: "alice",
		Items: []orders.Item{
			{SKU: "A-1", Qty: 2, PriceCents: 500},
			{SKU: "B-2", Qty: 1, PriceCents: 1250},
		},
	}
}

// wantKind 断言错误被翻译成了指定的 apperr.Kind。
func wantKind(t *testing.T, err error, want apperr.Kind) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望返回 %v 错误，got nil", want)
	}
	kind, ok := apperr.KindOf(err)
	if !ok {
		t.Fatalf("错误没被翻译成 apperr.Error: %v", err)
	}
	if kind != want {
		t.Errorf("Kind = %v, want %v（err = %v）", kind, want, err)
	}
}

// ---------- Place ----------

func TestOrderSvc_PlaceHappyPath(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := ordersvc.New(repo)

	o := validOrder()
	if err := svc.Place(context.Background(), o); err != nil {
		t.Fatalf("Place: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo.Create 被调用了 %d 次, want 1", len(repo.created))
	}
	if o.ID == 0 {
		t.Error("Place 之后 o.ID 还是 0")
	}
}

// TestOrderSvc_PlaceRejectsEmptyItems 是今天分层的关键一条。
//
// ⭐ 「订单没有商品」在 repository 层是【合法】的 ——
// D13 的 TestOrders_GetOrderWithNoItems 专门证明了这一点（schema 没禁止它）。
//
// 所以这条规则【只能】由业务层来立。如果 service 不管，
// 系统就会安静地接受空订单，直到某天财务发现一堆 0 元订单。
func TestOrderSvc_PlaceRejectsEmptyItems(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := ordersvc.New(repo)

	o := &orders.Order{Customer: "alice"} // 没有 Items
	err := svc.Place(context.Background(), o)

	wantKind(t, err, apperr.KindInvalid)
	if len(repo.created) != 0 {
		t.Error("校验失败时不该调用 repo.Create —— 业务校验要在存储之前")
	}
}

func TestOrderSvc_PlaceValidation(t *testing.T) {
	t.Parallel()

	tooManyItems := make([]orders.Item, ordersvc.MaxItemsPerOrder+1)
	for i := range tooManyItems {
		tooManyItems[i] = orders.Item{SKU: string(rune('a'+i%26)) + string(rune('0'+i/26)), Qty: 1, PriceCents: 1}
	}

	tests := []struct {
		name  string
		order *orders.Order
	}{
		{"customer 为空", &orders.Order{Items: validOrder().Items}},
		{"customer 全是空白", &orders.Order{Customer: "   ", Items: validOrder().Items}},
		{"没有 items", &orders.Order{Customer: "a"}},
		{"items 超过上限", &orders.Order{Customer: "a", Items: tooManyItems}},
		{"SKU 为空", &orders.Order{Customer: "a", Items: []orders.Item{{SKU: "", Qty: 1, PriceCents: 1}}}},
		{"Qty 为 0", &orders.Order{Customer: "a", Items: []orders.Item{{SKU: "X", Qty: 0, PriceCents: 1}}}},
		{"Qty 为负", &orders.Order{Customer: "a", Items: []orders.Item{{SKU: "X", Qty: -1, PriceCents: 1}}}},
		{"Qty 超过上限", &orders.Order{Customer: "a", Items: []orders.Item{{SKU: "X", Qty: ordersvc.MaxQtyPerItem + 1, PriceCents: 1}}}},
		{"价格为负", &orders.Order{Customer: "a", Items: []orders.Item{{SKU: "X", Qty: 1, PriceCents: -1}}}},
		{"SKU 重复", &orders.Order{Customer: "a", Items: []orders.Item{
			{SKU: "X", Qty: 1, PriceCents: 1},
			{SKU: "X", Qty: 1, PriceCents: 1},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepo()
			err := ordersvc.New(repo).Place(context.Background(), tt.order)

			wantKind(t, err, apperr.KindInvalid)
			if len(repo.created) != 0 {
				t.Error("校验失败时不该调用 repo.Create")
			}
		})
	}
}

// TestOrderSvc_PlaceDuplicateSKUIsInvalidNotConflict 锁住一个语义选择。
//
// 数据库有 UNIQUE(order_id, sku)，撞上会返回 Conflict（409）。
// 但同一个请求体里自己带了重复 SKU，那是【请求本身有问题】—— 400 更准确。
//
// ⭐ 业务层先挡一道，就不用把这种事交给数据库约束去表达。
func TestOrderSvc_PlaceDuplicateSKUIsInvalidNotConflict(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	err := ordersvc.New(repo).Place(context.Background(), &orders.Order{
		Customer: "alice",
		Items: []orders.Item{
			{SKU: "DUP", Qty: 1, PriceCents: 100},
			{SKU: "DUP", Qty: 2, PriceCents: 200},
		},
	})

	kind, _ := apperr.KindOf(err)
	if kind == apperr.KindConflict {
		t.Error("同一个请求里重复 SKU 应该是 Invalid(400) 而不是 Conflict(409)\n" +
			"（Conflict 表示「和现有数据冲突」，这里是请求自己有问题）")
	}
	wantKind(t, err, apperr.KindInvalid)
}

// TestOrderSvc_PlaceRejectsDuplicateSKUAnywhere 抓「只比较相邻元素」的去重实现。
//
// ⚠️ 这条是补上来的 —— 原来 TestOrderSvc_PlaceValidation 里那个「SKU 重复」
// 用的是 [X X]，**相邻**的。一个只记住「上一个 SKU」的实现能过：
//
//	sku := ""
//	for _, it := range o.Items {
//		if it.SKU == sku { ... }   // ⚠️ 只抓得到挨着的
//		sku = it.SKU
//	}
//
// 实测这种实现：[A A] 拦住了，[A B A] 和 [A B C D A] 直接放行进了 repo。
//
// ⭐ 又一次「测试数据太乖」（D6）—— 重复元素恰好挨在一起，是最容易通过的排列。
// 正确实现要用 map 做 set。
func TestOrderSvc_PlaceRejectsDuplicateSKUAnywhere(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		skus    []string
		wantErr bool
	}{
		{"相邻重复", []string{"A", "A"}, true},
		{"隔一个", []string{"A", "B", "A"}, true},
		{"隔很远", []string{"A", "B", "C", "D", "A"}, true},
		{"首尾重复", []string{"X", "B", "C", "X"}, true},
		{"中间重复", []string{"A", "B", "C", "B", "E"}, true},
		{"全不重复", []string{"A", "B", "C"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			items := make([]orders.Item, 0, len(tt.skus))
			for _, sku := range tt.skus {
				items = append(items, orders.Item{SKU: sku, Qty: 1, PriceCents: 100})
			}

			repo := newFakeRepo()
			err := ordersvc.New(repo).Place(context.Background(),
				&orders.Order{Customer: "alice", Items: items})

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("%v 不该被拒绝: %v", tt.skus, err)
				}
				return
			}

			wantKind(t, err, apperr.KindInvalid)
			if len(repo.created) != 0 {
				t.Errorf("%v 有重复 SKU，却还是调用了 repo.Create\n"+
					"（⚠️ 去重要用 map 做 set，别只比较相邻元素）", tt.skus)
			}
		})
	}
}

// TestOrderSvc_PlaceErrorMessagesAreAccurate 检查错误消息说的是不是实话。
//
// ⚠️ 把「太小」和「太大」合成一个分支：
//
//	if it.Qty < 1 || it.Qty > MaxQtyPerItem {
//		return apperr.Invalid(fmt.Sprintf("Qty %d 超过最大值 %d", ...))
//	}
//
// Qty = -5 时用户收到「Qty -5 超过最大值 1000」—— **字面上就是错的**，
// 排查的人会往「是不是上限配小了」的方向想。
//
// ⭐ 这条也是补上来的：原来的测试只断言 Kind == Invalid，不看消息内容，
// 所以这种「分类对了但说明错了」的 bug 完全测不出来。
// （review 关注点：错误消息对线上排查是否友好）
func TestOrderSvc_PlaceErrorMessagesAreAccurate(t *testing.T) {
	t.Parallel()

	maxStr := strconv.Itoa(ordersvc.MaxQtyPerItem)

	tests := []struct {
		name    string
		qty     int
		wantIn  string // 消息里必须出现（那个出问题的值）
		wantOut string // 消息里【不该】出现
		why     string
	}{
		{
			name: "Qty 为 0", qty: 0, wantIn: "0", wantOut: maxStr,
			why: "问题是「太小」，消息却提到上限，等于说反了",
		},
		{
			name: "Qty 为负", qty: -5, wantIn: "-5", wantOut: maxStr,
			why: "同上",
		},
		{
			name: "Qty 超上限", qty: ordersvc.MaxQtyPerItem + 1, wantIn: maxStr,
			why: "「太大」时提到上限是对的",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepo()
			err := ordersvc.New(repo).Place(context.Background(), &orders.Order{
				Customer: "alice",
				Items:    []orders.Item{{SKU: "X", Qty: tt.qty, PriceCents: 100}},
			})
			wantKind(t, err, apperr.KindInvalid)

			_, msg := apperr.HTTPStatus(err)
			if !strings.Contains(msg, tt.wantIn) {
				t.Errorf("消息里应该带上出问题的值 %q，得到:\n  %s\n"+
					"（不带具体数值的错误，用户不知道自己传了什么）", tt.wantIn, msg)
			}
			if tt.wantOut != "" && strings.Contains(msg, tt.wantOut) {
				t.Errorf("消息里不该出现 %q：%s\n  得到: %s", tt.wantOut, tt.why, msg)
			}
		})
	}
}

// TestOrderSvc_PlacePropagatesRepoError 确认 repo 的错误原样往上传，不被吞掉。
func TestOrderSvc_PlacePropagatesRepoError(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.createErr = apperr.Conflict("SKU 已存在", nil)

	err := ordersvc.New(repo).Place(context.Background(), validOrder())
	wantKind(t, err, apperr.KindConflict)
}

// ---------- Get ----------

func TestOrderSvc_GetRejectsBadID(t *testing.T) {
	t.Parallel()

	for _, id := range []int64{0, -1, -999} {
		repo := newFakeRepo()
		_, err := ordersvc.New(repo).Get(context.Background(), id)
		wantKind(t, err, apperr.KindInvalid)
	}
}

func TestOrderSvc_GetPassesThrough(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	id := repo.seed(orders.StatusPending)

	got, err := ordersvc.New(repo).Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}

	_, err = ordersvc.New(repo).Get(context.Background(), 99999)
	wantKind(t, err, apperr.KindNotFound)
}

// ---------- List ----------

// TestOrderSvc_ListRejectsUnknownStatus 锁住「非法过滤值要报错，不能返回空列表」。
//
// ⚠️ 返回空列表会让调用方以为「确实没有这种订单」，
// 而真实原因是他把 status 拼错了。⭐ 静默的空结果比报错更难查。
func TestOrderSvc_ListRejectsUnknownStatus(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"deleted", "PENDING", "pending ", ""} {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepo()
			_, err := ordersvc.New(repo).List(context.Background(),
				orders.ListFilter{Status: &bad})
			wantKind(t, err, apperr.KindInvalid)
		})
	}
}

func TestOrderSvc_ListAcceptsValidStatus(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{
		orders.StatusPending, orders.StatusPaid,
		orders.StatusShipped, orders.StatusCancelled,
	} {
		repo := newFakeRepo()
		if _, err := ordersvc.New(repo).List(context.Background(),
			orders.ListFilter{Status: &ok}); err != nil {
			t.Errorf("status=%q 应该是合法的: %v", ok, err)
		}
	}
}

func TestOrderSvc_ListNoFilterIsFine(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	repo.seed(orders.StatusPending)

	got, err := ordersvc.New(repo).List(context.Background(), orders.ListFilter{})
	if err != nil {
		t.Fatalf("不带过滤条件的 List 不该报错: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("拿到 %d 条, want 1", len(got))
	}
}

// ---------- 状态机 ----------

// TestOrderSvc_StateMachine 是今天业务逻辑的核心。
//
// ⭐ 这张表就是需求。任何一格改了，都应该有测试变红。
func TestOrderSvc_StateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from    string
		action  string
		wantErr bool
		wantTo  string
	}{
		{orders.StatusPending, "pay", false, orders.StatusPaid},
		{orders.StatusPending, "cancel", false, orders.StatusCancelled},
		{orders.StatusPaid, "cancel", false, orders.StatusCancelled},

		{orders.StatusPaid, "pay", true, ""},       // 已经支付过
		{orders.StatusShipped, "pay", true, ""},    // 已发货
		{orders.StatusShipped, "cancel", true, ""}, // 已发货不能取消
		{orders.StatusCancelled, "pay", true, ""},  // 已取消
		{orders.StatusCancelled, "cancel", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_"+tt.action, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepo()
			id := repo.seed(tt.from)
			svc := ordersvc.New(repo)

			var err error
			if tt.action == "pay" {
				err = svc.Pay(context.Background(), id)
			} else {
				err = svc.Cancel(context.Background(), id)
			}

			if tt.wantErr {
				wantKind(t, err, apperr.KindConflict)
				// ⚠️ 非法转换不该真的去改数据库
				if repo.updateCalled != 0 {
					t.Errorf("非法转换却调用了 %d 次 UpdateStatus —— "+
						"状态检查要在写库之前", repo.updateCalled)
				}
				return
			}

			if err != nil {
				t.Fatalf("%s → %s 应该允许: %v", tt.from, tt.action, err)
			}
			if repo.updateCalled != 1 {
				t.Fatalf("UpdateStatus 被调用了 %d 次, want 1", repo.updateCalled)
			}
			if got := repo.updatedTo[0]; got != tt.wantTo {
				t.Errorf("改成了 %q, want %q", got, tt.wantTo)
			}
		})
	}
}

// TestOrderSvc_TransitionErrorMentionsBothStates 检查错误消息的可排查性。
//
// ⚠️ 只说「操作不允许」的话，用户和排查的人都不知道发生了什么。
func TestOrderSvc_TransitionErrorMentionsBothStates(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	id := repo.seed(orders.StatusShipped)

	err := ordersvc.New(repo).Cancel(context.Background(), id)
	if err == nil {
		t.Fatal("shipped 不该能取消")
	}
	_, msg := apperr.HTTPStatus(err)

	for _, want := range []string{orders.StatusShipped, orders.StatusCancelled} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误消息里应该同时出现当前状态和目标状态，缺少 %q:\n  %s\n"+
				"（只说「操作不允许」的话，没人知道发生了什么）", want, msg)
		}
	}
}

func TestOrderSvc_TransitionOnMissingOrder(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := ordersvc.New(repo)

	wantKind(t, svc.Pay(context.Background(), 99999), apperr.KindNotFound)
	wantKind(t, svc.Cancel(context.Background(), 99999), apperr.KindNotFound)

	if repo.updateCalled != 0 {
		t.Error("订单不存在时不该调用 UpdateStatus")
	}
}

func TestOrderSvc_TransitionRejectsBadID(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	svc := ordersvc.New(repo)

	wantKind(t, svc.Pay(context.Background(), 0), apperr.KindInvalid)
	wantKind(t, svc.Cancel(context.Background(), -1), apperr.KindInvalid)
}

// TestOrderSvc_TransitionPropagatesRepoError 确认写库失败不会被当成成功。
func TestOrderSvc_TransitionPropagatesRepoError(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	id := repo.seed(orders.StatusPending)
	repo.updateErr = errors.New("连接断了")

	err := ordersvc.New(repo).Pay(context.Background(), id)
	if err == nil {
		t.Fatal("repo 报错时 Pay 应该失败")
	}
}
