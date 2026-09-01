// Package apperr 定义领域错误，以及它到传输层（HTTP）的映射。
//
// 设计目标（D12 §5）：让 handler 不需要知道「什么错该返回什么状态码」。
//
//	repository  把底层错误（sql.ErrNoRows、网络错误）【翻译】成 apperr.Error
//	service     用 %w 加上下文，【不改】Kind
//	handler     调一次 HTTPStatus，拿到状态码 + 用户可见消息
//
// ⚠️ 本包【不 import net/http】—— Kind 是领域概念（「找不到」），
// HTTP 状态码是传输细节。映射函数放在 http.go 里，领域部分保持干净。
package apperr

import (
	"errors"
	"fmt"
)

// Kind 是错误的类别。它描述「出了什么性质的问题」，不描述怎么告诉客户端。
type Kind int

// 错误类别。零值是 KindInternal —— ⭐ 这是刻意的：
// 忘了指定类别时，默认按「服务端内部错误」处理，而不是默默当成 200/404。
const (
	KindInternal     Kind = iota // 服务端自己的问题
	KindNotFound                 // 请求的资源不存在
	KindInvalid                  // 请求本身有问题（参数、格式）
	KindConflict                 // 和当前状态冲突（重复创建、版本冲突）
	KindUnauthorized             // 没有身份或身份无效
	KindForbidden                // 有身份，但没权限
	KindRateLimited              // 请求太频繁
)

// String 让 Kind 在日志里可读。
//
// TODO(D12)：实现我。
//
// 返回 "internal" / "not_found" / "invalid" / "conflict" /
// "unauthorized" / "forbidden" / "rate_limited"；
// 未知的 Kind 返回 fmt.Sprintf("kind(%d)", int(k))。
func (k Kind) String() string {
	switch k {
	case KindInternal:
		return "internal"
	case KindNotFound:
		return "not_found"
	case KindInvalid:
		return "invalid"
	case KindConflict:
		return "conflict"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindRateLimited:
		return "rate_limited"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// Error 是领域错误。
//
// ⭐ 关键在于【两个消息】：
//
//	Message  给【用户】看 —— 不含任何内部细节
//	Err      给【日志】看 —— 含全部细节（SQL、IP、堆栈…）
//
// 这两者混在一起，就会出现「把 dial tcp 10.0.0.5:5432: connection refused
// 返回给客户端」这种事故。
type Error struct {
	// Kind 决定这个错误在边界处被翻译成什么。
	Kind Kind
	// Message 是面向用户的说明。⚠️ 它会被原样返回给客户端。
	Message string
	// Err 是被包装的底层错误，可以为 nil。
	Err error
}

// Error 实现 error 接口。
//
// TODO(D12)：实现我。
//
// 格式：
//
//	Err != nil  →  "<Message>: <Err.Error()>"
//	Err == nil  →  "<Message>"
//
// ⚠️ 注意这个字符串是给【开发者】看的（日志、%v），所以带上 Err 是对的。
// 面向用户的那一份由 HTTPStatus 单独返回，两者不要混。
func (e *Error) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Message, e.Err.Error())
}

// Unwrap 让 errors.Is / errors.As 能穿透到底层错误（D2）。
//
// TODO(D12)：实现我。
//
// ⚠️ 少了这个方法，`errors.Is(err, sql.ErrNoRows)` 在包装之后就找不到了 ——
// 整条 %w 链在这里断掉。
func (e *Error) Unwrap() error {
	return e.Err
}

// ---------- 构造函数 ----------
//
// 每个类别一个构造函数，比 &Error{Kind: KindNotFound, ...} 好读，
// 也让「新增一个类别」这件事有唯一的入口。
//
// TODO(D12)：把下面六个都实现掉（每个都是一行）。

// NotFound 表示请求的资源不存在。
func NotFound(msg string, err error) *Error {
	return &Error{Kind: KindNotFound, Message: msg, Err: err}
}

// Invalid 表示请求本身有问题。
func Invalid(msg string, err error) *Error {
	return &Error{Kind: KindInvalid, Message: msg, Err: err}
}

// Conflict 表示和当前状态冲突。
func Conflict(msg string, err error) *Error {
	return &Error{Kind: KindConflict, Message: msg, Err: err}
}

// Unauthorized 表示没有身份或身份无效。
func Unauthorized(msg string, err error) *Error {
	return &Error{Kind: KindUnauthorized, Message: msg, Err: err}
}

// Forbidden 表示有身份但没权限。
func Forbidden(msg string, err error) *Error {
	return &Error{Kind: KindForbidden, Message: msg, Err: err}
}

// Internal 表示服务端自己的问题。
func Internal(msg string, err error) *Error {
	return &Error{Kind: KindInternal, Message: msg, Err: err}
}

// ---------- 查询 ----------

// KindOf 返回 err 链上最外层 *Error 的 Kind。
//
// TODO(D12)：实现我。
//
// 规则：
//
//   - err 为 nil                → 返回 KindInternal, false
//   - 链上找得到 *Error         → 返回它的 Kind, true
//   - 找不到（普通 error）      → 返回 KindInternal, false
//
// ⚠️ 必须用 errors.As，不能用类型断言 `e, ok := err.(*Error)` ——
// service 层会用 fmt.Errorf("...: %w", err) 包一层，断言当场就失效了。
//
// ⚠️ 「最外层」是 errors.As 的天然行为，也是我们想要的：
// 如果外层刻意用 apperr.Internal 重新包装了一个 NotFound，
// 说明外层【决定】把它降级成内部错误，应该听外层的。
func KindOf(err error) (Kind, bool) {
	var e *Error
	if ok := errors.As(err, &e); ok {
		return e.Kind, true
	}
	return KindInternal, false
}
