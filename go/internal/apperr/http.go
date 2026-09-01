package apperr

import "net/http"

// GenericMessage 是兜底的用户可见消息。
//
// ⭐ 它【故意】不含任何细节。当错误不是 *Error（也就是没人主动想过
// 「这个错该怎么告诉用户」）时，宁可什么都不说，也不要把内部信息漏出去。
const GenericMessage = "服务暂时不可用，请稍后重试"

// HTTPStatus 把任意 error 映射成 HTTP 状态码和【用户可见】的消息。
//
// 这是领域到传输的唯一边界 —— handler 里应该只有这一行，
// 不该出现任何 errors.Is(err, sql.ErrNoRows) 之类的判断。
//
// TODO(D12)：实现我。
//
// 规则，按顺序：
//
//  1. err == nil                    → (200, "")
//  2. context.Canceled              → (499, "请求已取消")
//     ⭐ 499 是 nginx 的非标准码「客户端主动断开」。客户端都走了，
//     写 500 只会污染你的错误率监控（D10 那套 ctx 的下游影响）。
//  3. context.DeadlineExceeded      → (504, "处理超时")
//  4. 链上有 *Error                 → (Kind 对应的状态码, 那个 Error 的 Message)
//     ⚠️ 如果 Message 是空字符串，用 GenericMessage 兜底
//  5. 其他                          → (500, GenericMessage)
//
// Kind 到状态码：
//
//	KindNotFound      → 404
//	KindInvalid       → 400
//	KindConflict      → 409
//	KindUnauthorized  → 401
//	KindForbidden     → 403
//	KindRateLimited   → 429
//	KindInternal      → 500
//	未知 Kind          → 500
//
// ⚠️ 第 4 条最容易做错的地方：返回 e.Error()（含 Err 的细节）而不是 e.Message。
// 测试会专门验证底层错误的内容【不】出现在返回的消息里。
//
// ⚠️ 顺序也有讲究：ctx 的两条要排在 *Error 之前还是之后？
// 想清楚「repository 把 ctx 超时包装成了 apperr.Internal」时你希望返回什么。
// 这题没有唯一答案，但你要能说出你选的理由 —— review 时我会问。
func HTTPStatus(err error) (status int, message string) {
	panic("TODO(D12): 实现 HTTPStatus")
}

// StatusClientClosedRequest 是 nginx 定义的 499，标准库里没有。
const StatusClientClosedRequest = 499

var _ = http.StatusNotFound // 实现完可以删掉
