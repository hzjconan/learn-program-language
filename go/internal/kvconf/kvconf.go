// Package kvconf 解析 key=value 格式的配置文本。
//
// 这个包的重点不是解析本身（逻辑很简单），而是错误处理：
// 每一个解析失败都必须携带行号，且调用方能用 errors.As 把它取出来。
package kvconf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// 解析过程中可能出现的哨兵错误（sentinel error）。
//
// 哨兵错误是包级变量，调用方用 errors.Is 和它们比对。
// 命名必须是 ErrXxx（revive 的 error-naming 规则强制）；
// 消息小写开头、结尾不加标点 —— 因为它们会被拼进更长的错误串里。
var (
	// ErrMissingSeparator 表示该行没有 '=' 分隔符。
	ErrMissingSeparator = errors.New("缺少 '=' 分隔符")
	// ErrEmptyKey 表示 '=' 左边为空。
	ErrEmptyKey = errors.New("键为空")
	// ErrDuplicateKey 表示同一个键出现了多次。
	ErrDuplicateKey = errors.New("键重复")
)

// ParseError 描述一次解析失败，携带出错的行号与原始行内容。
//
// 这就是「需要携带结构化数据时用自定义错误类型」的典型：
// 调用方光知道「解析失败了」没用，它要知道是第几行。
type ParseError struct {
	// Line 是出错的行号，从 1 开始，包含被跳过的空行和注释行。
	Line int
	// Text 是出错那一行的原始内容（未做 trim）。
	Text string
	// Err 是底层原因，取值为本包的某个哨兵错误。
	Err error
}

// Error 实现 error 接口。
//
// 格式固定为：`第 3 行: 缺少 '=' 分隔符 (原文: "abc")`
// 注意原文用 %q 输出 —— 带引号才能看见首尾空格和空字符串。
//
// TODO(D2 主练习)：实现我。
func (e *ParseError) Error() string {
	return fmt.Sprintf("第 %d 行: %v (原文: %q)", e.Line, e.Err, e.Text)
}

// Unwrap 返回底层错误，让 errors.Is / errors.As 能穿透本类型继续往下找。
//
// 没有这个方法，错误链就在 ParseError 这里断掉，
// 调用方的 errors.Is(err, ErrEmptyKey) 永远返回 false。
//
// TODO(D2 主练习)：实现我。
func (e *ParseError) Unwrap() error {
	return e.Err
}

// Parse 从 r 读取 key=value 配置，返回解析结果。
//
// 语法规则：
//   - 每行一个 key=value，按【第一个】'=' 分割，所以 value 里可以再出现 '='。
//   - key 和 value 各自做 TrimSpace；value 允许为空字符串。
//   - 空行和纯空白行跳过。
//   - TrimSpace 后以 '#' 开头的行是注释，跳过。
//   - key 为空（如 "=v"）报 ErrEmptyKey；整行没有 '=' 报 ErrMissingSeparator；
//     同一个 key 出现两次报 ErrDuplicateKey。
//
// 行号从 1 开始，且【包含】被跳过的空行与注释行 —— 报错时人要能直接跳到编辑器的那一行。
//
// 遇到第一个错误立即返回 (nil, *ParseError)，不继续解析。
// 读取 r 本身出错时，包装后返回（此时不是 *ParseError）。
//
// TODO(D2 主练习)：实现我。
//
// 提示：bufio.NewScanner(r) + scanner.Scan() / scanner.Text() 是标准姿势，
// 循环结束后别忘了检查 scanner.Err()。
func Parse(r io.Reader) (map[string]string, error) {
	result := make(map[string]string)
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		rawKey, _, found := strings.Cut(line, "=")
		if !found {
			return nil, &ParseError{Line: lineNum, Text: line, Err: ErrMissingSeparator}
		}
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return nil, &ParseError{Line: lineNum, Text: line, Err: ErrEmptyKey}
		}
		if _, ok := result[key]; ok {
			return nil, &ParseError{Line: lineNum, Text: line, Err: ErrDuplicateKey}
		}
		result[key] = strings.TrimSpace(line[idx+1:])
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取配置: %w", err)
	}

	return result, nil
}

// ParseFile 读取并解析 path 指向的配置文件。
//
// 两条硬要求，测试会逐条验：
//
//  1. 打开文件失败时，调用方必须能用 errors.Is(err, fs.ErrNotExist) 判断出
//     「文件不存在」—— 这意味着你的包装链一路 %w 到 os.Open 返回的错误，
//     中间任何一层用了 %v 都会把链切断。
//  2. 解析失败时，调用方必须能用 errors.As 取到 *ParseError 和它的行号。
//
// 错误消息里要带上 path，否则调用方不知道是哪个文件出的问题。
//
// TODO(D2 主练习)：实现我。
//
// 提示：文件要 defer 关闭；只读文件用 defer func() { _ = f.Close() }() 即可，
// 但想想为什么写文件时这样不行（讲义 §5 末尾）。
func ParseFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close() //nolint:errcheck // 只读文件，关闭错误无需处理
	}()
	m, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}
