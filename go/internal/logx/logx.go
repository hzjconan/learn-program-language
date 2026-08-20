// Package logx 解析并聚合结构化日志，是 D7 的周综合项目。
//
// # 日志格式
//
// 每行五个部分，空格分隔，消息是剩下的全部：
//
//	2026-08-18T10:23:45Z INFO api-gateway 15 GET /users/42 -> 200
//	│                    │    │           │  │
//	│                    │    │           │  └ 消息（本行剩下的全部，可含空格，也可以为空）
//	│                    │    │           └─── 耗时，毫秒（非负整数）
//	│                    │    └─────────────── 服务名（非空、不含空格）
//	│                    └──────────────────── 级别（DEBUG/INFO/WARN/ERROR，大小写不敏感）
//	└───────────────────────────────────────── 时间（RFC3339）
//
// # 这个文件覆盖的知识点
//
//   - D1：具名类型 + iota + String()
//   - D2：错误包装、带定位信息的自定义错误、errors.As / Unwrap
//   - D3：字符串切分、不共享底层数组
package logx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ErrMalformed 表示一行的整体结构不对（字段数量不够）。
//
// 哨兵错误：调用方用 errors.Is(err, ErrMalformed) 判断（QA #8）。
var ErrMalformed = errors.New("logx: 行格式不正确")

// Level 是日志级别。
//
// 用具名类型而不是 string，这样级别可以比较大小（LevelWarn > LevelInfo），
// 也让编译器帮你挡住「把服务名传进级别参数」这类错误（D1）。
type Level int

// 四个日志级别，按严重程度升序。
//
// 用 iota 而不是手写 0/1/2/3 —— 中间插入新级别时不用重编号（D1）。
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String 实现 fmt.Stringer，返回大写的级别名。
//
// TODO(D7)：实现我。
//
// 未知的取值返回 `Level(7)` 这种形式 —— 这是标准库的惯例（对照 time.Month）。
// 别返回空字符串：日志里出现一个空白比出现 Level(7) 难查得多。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "Level(" + strconv.Itoa(int(l)) + ")"
	}
}

// ParseLevel 把字符串解析成 Level，大小写不敏感。
//
// TODO(D7)：实现我。
//
// 无法识别时返回 LevelDebug 和一个错误 —— 错误消息里要带上那个非法的字符串本身。
func ParseLevel(s string) (Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	default:
		return LevelDebug, errors.New("未知的级别 " + s)
	}
}

// Entry 是一条解析成功的日志。
//
// 全部字段都是值类型，没有 slice / map —— 所以拷贝它是安全的深拷贝（D4 §1）。
// 这是有意的设计：Entry 会被大量拷贝进 Report，不希望出现别名共享。
type Entry struct {
	// Time 是日志时间。
	Time time.Time
	// Level 是日志级别。
	Level Level
	// Service 是产生这条日志的服务名。
	Service string
	// Latency 是这次操作的耗时。
	//
	// 用 time.Duration 而不是 int：类型自带单位，String() 也好看（D1）。
	Latency time.Duration
	// Message 是日志正文，可以为空。
	Message string
}

// ParseError 描述某一行为什么解析失败。
//
// 带**行号**和**字段名** —— 这是 D2 kvconf 那道题的延续，也是我每次 review
// 都会盯的可观测性：线上看到「解析失败」而不知道是哪一行哪个字段，等于没报错。
type ParseError struct {
	// Line 是出错的行号，从 1 开始。
	Line int
	// Field 是出问题的字段名：time / level / service / latency / format。
	Field string
	// Err 是底层错误。
	Err error
}

// Error 实现 error。
//
// TODO(D7)：实现我。
//
// 格式固定为：`第 12 行 latency 字段: 无效的耗时 "abc"`
// 注意用指针接收者 —— 错误类型要身份语义（D4 §3）。
func (e *ParseError) Error() string {
	return fmt.Sprintf("第 %v 行 %v 字段: %v", e.Line, e.Field, e.Err.Error())
}

// Unwrap 让 errors.Is / errors.As 能穿透到底层错误。
//
// TODO(D7)：实现我。
func (e *ParseError) Unwrap() error {
	return e.Err
}

// ParseLine 解析一行日志。lineNo 只用于错误消息，从 1 开始。
//
// TODO(D7)：实现我。
//
// 失败时返回的错误必须是 *ParseError（调用方要用 errors.As 拿 Line 和 Field），
// 且 Unwrap 之后能被 errors.Is 识别出底层原因。
//
// 规则：
//   - 切出来不足 4 段 → Field="format"，包装 ErrMalformed
//   - **正好 4 段 → 合法，消息为空字符串**（`... INFO cache 2` 是一条有效日志）
//   - 时间不合法 → Field="time"
//   - 级别不认识 → Field="level"
//   - 服务名为空 → Field="service"
//   - 耗时不是非负整数 → Field="latency"
//
// 提示：strings.SplitN(s, " ", 5) 一次切出至多五段，第五段就是整条消息（D3）。
// 想想为什么不能用 strings.Split —— 消息里含空格时会被切碎。
func ParseLine(lineNo int, s string) (Entry, error) {
	parts := strings.SplitN(s, " ", 5)
	if len(parts) < 4 {
		return Entry{}, &ParseError{
			Line:  lineNo,
			Field: "format",
			Err:   ErrMalformed,
		}
	}

	result := Entry{}
	timeStr := parts[0]
	levelStr := parts[1]
	serviceStr := parts[2]
	latencyStr := parts[3]

	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return Entry{}, &ParseError{
			Line:  lineNo,
			Field: "time",
			Err:   fmt.Errorf("时间格式不正确 %q: %w", timeStr, err),
		}
	}
	result.Time = t

	l, err := ParseLevel(levelStr)
	if err != nil {
		return Entry{}, &ParseError{
			Line:  lineNo,
			Field: "level",
			Err:   err,
		}
	}
	result.Level = l

	if serviceStr == "" {
		return Entry{}, &ParseError{
			Line:  lineNo,
			Field: "service",
			Err:   errors.New("服务名不能为空"),
		}
	}
	result.Service = serviceStr

	v, err := strconv.Atoi(latencyStr)
	if err != nil {
		return Entry{}, &ParseError{Line: lineNo, Field: "latency",
			Err: fmt.Errorf("耗时 %q 不是整数: %w", latencyStr, err)}
	}
	if v < 0 {
		return Entry{}, &ParseError{Line: lineNo, Field: "latency",
			Err: fmt.Errorf("耗时不能为负: %d", v)}
	}
	result.Latency = time.Duration(v) * time.Millisecond

	if len(parts) == 5 {
		result.Message = parts[4]
	}
	return result, nil
}
