package apperr_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
)

// sentinel 模拟存储层的底层错误（现实里是 sql.ErrNoRows 之类）。
var errNoRows = errors.New("sql: no rows in result set")

// secret 是「绝对不能出现在用户可见消息里」的内部细节。
// 每个泄漏测试都拿它做探针。
const secret = "dial tcp 10.0.0.5:5432: connection refused"

// ---------- Kind ----------

func TestAppErr_KindString(t *testing.T) {
	tests := []struct {
		kind apperr.Kind
		want string
	}{
		{apperr.KindInternal, "internal"},
		{apperr.KindNotFound, "not_found"},
		{apperr.KindInvalid, "invalid"},
		{apperr.KindConflict, "conflict"},
		{apperr.KindUnauthorized, "unauthorized"},
		{apperr.KindForbidden, "forbidden"},
		{apperr.KindRateLimited, "rate_limited"},
		{apperr.Kind(99), "kind(99)"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", int(tt.kind), got, tt.want)
		}
	}
}

// TestAppErr_KindZeroValueIsInternal 锁住「零值必须是最保守的那个」。
//
// 如果有人调整常量顺序，把 KindNotFound 排到第一个，那么
// &Error{Message: "x"}（忘了写 Kind）就会静默变成 404 —— 一个本该
// 报警的内部错误变成了「资源不存在」，监控上完全看不见。
func TestAppErr_KindZeroValueIsInternal(t *testing.T) {
	var zero apperr.Kind
	if zero != apperr.KindInternal {
		t.Errorf("Kind 的零值是 %v，必须是 KindInternal\n"+
			"（忘写 Kind 时应该按最保守的「内部错误」处理，而不是变成 404/401）", zero)
	}

	var e apperr.Error // 零值 Error
	if status, _ := apperr.HTTPStatus(&e); status != http.StatusInternalServerError {
		t.Errorf("零值 Error 的状态码 = %d, want 500", status)
	}
}

// ---------- 构造函数 ----------

func TestAppErr_Constructors(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string, error) *apperr.Error
		want apperr.Kind
	}{
		{"NotFound", apperr.NotFound, apperr.KindNotFound},
		{"Invalid", apperr.Invalid, apperr.KindInvalid},
		{"Conflict", apperr.Conflict, apperr.KindConflict},
		{"Unauthorized", apperr.Unauthorized, apperr.KindUnauthorized},
		{"Forbidden", apperr.Forbidden, apperr.KindForbidden},
		{"Internal", apperr.Internal, apperr.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.fn("给用户的话", errNoRows)
			if e.Kind != tt.want {
				t.Errorf("Kind = %v, want %v", e.Kind, tt.want)
			}
			if e.Message != "给用户的话" {
				t.Errorf("Message = %q, want %q", e.Message, "给用户的话")
			}
			if !errors.Is(e.Err, errNoRows) {
				t.Errorf("Err = %v, want 包住 errNoRows", e.Err)
			}
		})
	}
}

// ---------- Error / Unwrap ----------

func TestAppErr_ErrorString(t *testing.T) {
	withErr := apperr.NotFound("找不到商品", errNoRows)
	if got, want := withErr.Error(), "找不到商品: "+errNoRows.Error(); got != want {
		t.Errorf("Error() = %q\nwant %q", got, want)
	}

	// Err 为 nil 时不能出现多余的冒号或 "<nil>"
	bare := apperr.Invalid("参数不合法", nil)
	got := bare.Error()
	if got != "参数不合法" {
		t.Errorf("Err 为 nil 时 Error() = %q, want %q\n"+
			"（别无条件拼 \": \" + e.Err.Error() —— nil 会 panic 或打出 <nil>）", got, "参数不合法")
	}
}

// TestAppErr_UnwrapKeepsChain 抓的是【忘了实现 Unwrap】。
//
// 少了 Unwrap，errors.Is 就穿不透，上层再也认不出底层的哨兵错误。
func TestAppErr_UnwrapKeepsChain(t *testing.T) {
	e := apperr.NotFound("找不到商品", errNoRows)

	if !errors.Is(e, errNoRows) {
		t.Error("errors.Is(e, errNoRows) = false\n" +
			"（*Error 要实现 Unwrap() error，否则 %w 链在这里断掉 —— D2）")
	}

	// 再套一层 fmt.Errorf，链仍然要通
	wrapped := fmt.Errorf("购买 sku-1: %w", e)
	if !errors.Is(wrapped, errNoRows) {
		t.Error("多包一层之后 errors.Is 就找不到 errNoRows 了")
	}

	// Err 为 nil 时 Unwrap 应该返回 nil，不能 panic
	if got := errors.Unwrap(apperr.Invalid("x", nil)); got != nil {
		t.Errorf("Err 为 nil 时 Unwrap() = %v, want nil", got)
	}
}

// ---------- KindOf ----------

// TestAppErr_KindOfSeesThroughWrapping 抓的是【用类型断言代替 errors.As】。
//
// ⚠️ 这是最容易写错的一处：`e, ok := err.(*Error)` 在没包装时能过测试，
// 一旦 service 层加了 fmt.Errorf("...: %w", err) 就当场失效。
// 所以测试必须包上好几层。
func TestAppErr_KindOfSeesThroughWrapping(t *testing.T) {
	base := apperr.NotFound("找不到商品", errNoRows)

	tests := []struct {
		name string
		err  error
	}{
		{"没包装", base},
		{"包一层", fmt.Errorf("repo.Get: %w", base)},
		{"包三层", fmt.Errorf("handler: %w", fmt.Errorf("service.Buy: %w", fmt.Errorf("repo.Get: %w", base)))},
		{"用 errors.Join 包住", errors.Join(base)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := apperr.KindOf(tt.err)
			if !ok {
				t.Fatalf("KindOf 没找到 *Error\n"+
					"（用 errors.As，不要用类型断言 err.(*Error)）\nerr = %v", tt.err)
			}
			if kind != apperr.KindNotFound {
				t.Errorf("Kind = %v, want KindNotFound", kind)
			}
		})
	}
}

func TestAppErr_KindOfNonAppError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"普通 error", errNoRows},
		{"包装过的普通 error", fmt.Errorf("x: %w", errNoRows)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := apperr.KindOf(tt.err)
			if ok {
				t.Errorf("KindOf(%v) 的 ok = true, want false", tt.err)
			}
			if kind != apperr.KindInternal {
				t.Errorf("找不到时应该返回 KindInternal（最保守），得到 %v", kind)
			}
		})
	}
}

// TestAppErr_KindOfPrefersOutermost 锁住「外层重新分类时听外层的」。
//
// 场景：repository 返回 NotFound，但 service 判断这在业务上属于数据不一致，
// 刻意用 Internal 重新包装。这时候不该再返回 404。
func TestAppErr_KindOfPrefersOutermost(t *testing.T) {
	inner := apperr.NotFound("找不到商品", errNoRows)
	outer := apperr.Internal("库存数据不一致", inner)

	kind, ok := apperr.KindOf(outer)
	if !ok {
		t.Fatal("KindOf 应该找到 *Error")
	}
	if kind != apperr.KindInternal {
		t.Errorf("Kind = %v, want KindInternal\n"+
			"（外层刻意重新分类了，errors.As 找最外层正是我们要的行为）", kind)
	}

	// 但内层的链仍然要能穿透
	if !errors.Is(outer, errNoRows) {
		t.Error("重新包装之后，到底层哨兵错误的链断了")
	}
}

// ---------- HTTPStatus ----------

func TestAppErr_HTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"nil", nil, http.StatusOK, ""},
		{"NotFound", apperr.NotFound("找不到商品", errNoRows), http.StatusNotFound, "找不到商品"},
		{"Invalid", apperr.Invalid("价格必须为正", nil), http.StatusBadRequest, "价格必须为正"},
		{"Conflict", apperr.Conflict("商品已存在", nil), http.StatusConflict, "商品已存在"},
		{"Unauthorized", apperr.Unauthorized("请先登录", nil), http.StatusUnauthorized, "请先登录"},
		{"Forbidden", apperr.Forbidden("无权访问", nil), http.StatusForbidden, "无权访问"},
		{"Internal", apperr.Internal("写库失败", errNoRows), http.StatusInternalServerError, "写库失败"},
		{"RateLimited", &apperr.Error{Kind: apperr.KindRateLimited, Message: "太快了"}, http.StatusTooManyRequests, "太快了"},
		{"未知 Kind", &apperr.Error{Kind: apperr.Kind(99), Message: "?"}, http.StatusInternalServerError, "?"},
		{"普通 error", errNoRows, http.StatusInternalServerError, apperr.GenericMessage},
		{"包装过的领域错误", fmt.Errorf("service.Buy: %w", apperr.NotFound("找不到商品", nil)), http.StatusNotFound, "找不到商品"},
		{"Message 为空", apperr.NotFound("", errNoRows), http.StatusNotFound, apperr.GenericMessage},
		{"ctx 取消", context.Canceled, apperr.StatusClientClosedRequest, "请求已取消"},
		{"ctx 超时", context.DeadlineExceeded, http.StatusGatewayTimeout, "处理超时"},
		{"包装过的 ctx 超时", fmt.Errorf("查库: %w", context.DeadlineExceeded), http.StatusGatewayTimeout, "处理超时"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := apperr.HTTPStatus(tt.err)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if msg != tt.wantMsg {
				t.Errorf("message = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

// TestAppErr_HTTPStatusDoesNotLeakInternals 是今天最重要的一条。
//
// ⚠️ 最常见的写法是 `return status, err.Error()` —— 测试全绿，
// 然后生产环境把数据库地址、SQL 语句、内网 IP 全返回给了客户端。
//
// 探针：把 secret 塞进 Err，断言它【不】出现在返回的消息里。
func TestAppErr_HTTPStatusDoesNotLeakInternals(t *testing.T) {
	internal := errors.New(secret)

	tests := []struct {
		name string
		err  error
	}{
		{"NotFound 包着内部错误", apperr.NotFound("找不到商品", internal)},
		{"Internal 包着内部错误", apperr.Internal("查询失败", internal)},
		{"再被 fmt.Errorf 包一层", fmt.Errorf("service.Buy: %w", apperr.Internal("查询失败", internal))},
		{"裸的内部错误", internal},
		{"Message 为空", apperr.Internal("", internal)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, msg := apperr.HTTPStatus(tt.err)
			if strings.Contains(msg, secret) {
				t.Errorf("用户可见消息里泄漏了内部细节！\n"+
					"got:  %q\n"+
					"（返回 e.Message，不是 e.Error() —— 后者按设计就包含 Err 的全部内容）", msg)
			}
			// 顺带确认：10.0.0.5 这样的片段也不能漏
			for _, frag := range []string{"10.0.0.5", "5432", "dial tcp"} {
				if strings.Contains(msg, frag) {
					t.Errorf("消息里出现了内部片段 %q：%q", frag, msg)
				}
			}
		})
	}
}

// TestAppErr_ErrorStringKeepsInternals 是上一条的【反面】。
//
// 给日志看的那一份必须【保留】全部细节 —— 否则线上出问题你什么都查不到。
// 两条测试合起来才锁住「两个消息」的设计（review 关注点：可观测性）。
func TestAppErr_ErrorStringKeepsInternals(t *testing.T) {
	internal := errors.New(secret)
	e := apperr.Internal("查询失败", internal)

	if !strings.Contains(e.Error(), secret) {
		t.Errorf("Error() = %q\n"+
			"必须包含底层错误的细节 —— 这一份是给日志/开发者看的，"+
			"脱敏发生在 HTTPStatus 那一侧，不是这里", e.Error())
	}
	if !strings.Contains(e.Error(), "查询失败") {
		t.Errorf("Error() 也应该包含 Message，得到 %q", e.Error())
	}
}

// TestAppErr_HandlerNeedsNoDomainKnowledge 演示这套设计的收益：
// handler 里没有任何 errors.Is(err, sql.ErrNoRows)。
//
// 这条更像一份可执行的文档 —— review 时我们会对着它聊分层。
func TestAppErr_HandlerNeedsNoDomainKnowledge(t *testing.T) {
	// ① repository：把底层错误翻译成领域错误
	repoGet := func(id string) error {
		if id == "missing" {
			return apperr.NotFound("找不到这个商品", errNoRows)
		}
		return nil
	}
	// ② service：加上下文，不改 Kind
	serviceBuy := func(id string) error {
		if err := repoGet(id); err != nil {
			return fmt.Errorf("购买 %s: %w", id, err)
		}
		return nil
	}
	// ③ handler：只做映射
	handle := func(id string) (int, string) {
		if err := serviceBuy(id); err != nil {
			return apperr.HTTPStatus(err)
		}
		return http.StatusOK, ""
	}

	if status, msg := handle("missing"); status != http.StatusNotFound || msg != "找不到这个商品" {
		t.Errorf("handle(\"missing\") = %d, %q; want 404, \"找不到这个商品\"", status, msg)
	}
	if status, _ := handle("ok"); status != http.StatusOK {
		t.Errorf("handle(\"ok\") = %d, want 200", status)
	}
}
