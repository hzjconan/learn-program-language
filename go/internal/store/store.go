// Package store 是一个可插拔的记录存储抽象。
//
// 这是 D5 的主练习，要练四件事：
//
//  1. **小接口**：Getter / Putter 各一个方法，Store 由它俩组合出来（D5 §2）。
//  2. **依赖倒置**：CopyAll 只依赖接口，换任何实现都不用改它一行（D5 §1）。
//  3. **可选接口**：Count 用类型断言探测额外能力，探测不到就降级（D5 §5）。
//  4. **nil 接口的坑**：Save 里埋了雷，测试会专门抓（D5 §4）—— 这是今天的题眼。
//
// # 关于接口放在哪
//
// D5 §2 说「接口应该由使用方定义」。这个包里 Getter/Putter 和 MemStore 放在一起，
// 是因为本包自己就是使用方（CopyAll、Count 都在这儿）。
// 真实项目里如果只有别的包用，接口就该搬到那个包里去，而且通常不导出。
package store

import (
	"errors"
	"fmt"
	"slices"
)

// Record 是被存储的一条记录。
type Record struct {
	// ID 是主键，不能为空。
	ID string
	// Name 是展示名，不能为空。
	Name string
	// Tags 是标签，可以为空。
	Tags []string
}

// ErrNotFound 表示记录不存在。
//
// 它是【哨兵错误】：调用方用 errors.Is(err, ErrNotFound) 判断，而不是比字符串（QA #8）。
var ErrNotFound = errors.New("记录不存在")

// Getter 是「能按 ID 取一条记录」这个能力。
//
// 一个方法。小到几乎所有存储实现都能满足它 —— 这就是 D5 §2 说的
// 「接口越小，能塞进去的类型越多」。
type Getter interface {
	// Get 返回 id 对应的记录。不存在时返回的错误要满足 errors.Is(err, ErrNotFound)。
	Get(id string) (Record, error)
}

// Putter 是「能写入一条记录」这个能力。
type Putter interface {
	// Put 写入 r，同 ID 覆盖。
	Put(r Record) error
}

// Store 同时具备读写能力。
//
// 注意它是【组合】出来的，不是一开始就写一个大接口 —— 对照 D4 §4 的嵌入，
// 那里嵌的是 struct，这里嵌的是接口。
type Store interface {
	Getter
	Putter
}

// Counter 是一个【可选接口】：不是所有 Getter 都实现它。
//
// 这是 http.ResponseWriter 探测 http.Flusher 用的同一个模式 ——
// 基础能力放主接口里，增强能力靠断言探测（D5 §5）。
type Counter interface {
	// Len 返回当前记录条数。
	Len() int
}

// MemStore 是基于 map 的内存实现。
//
// 【零值可用】（D4 §1）：var m MemStore 之后可以直接 Put，不需要任何构造函数。
// 做到这点需要在 Put 里惰性初始化 map —— 因为 nil map 可读不可写（D3）。
type MemStore struct {
	records map[string]Record
}

// Get 实现 Getter。
//
// TODO(D5 主练习)：实现我。
//
// 记录不存在时返回的错误必须满足 errors.Is(err, ErrNotFound)，
// 并且**错误消息里要带上 id** —— 线上只看到「记录不存在」是没法排查的（D4 review）。
// 用 %w 包装：fmt.Errorf("... %q ...: %w", id, ErrNotFound)
func (m *MemStore) Get(id string) (Record, error) {
	r, ok := m.records[id]
	if !ok {
		return Record{}, fmt.Errorf("记录 %q 不存在: %w", id, ErrNotFound)
	}
	return r, nil
}

// Put 实现 Putter。
//
// TODO(D5 主练习)：实现我。
//
// 要点：惰性初始化 map，让零值 MemStore 可用。
func (m *MemStore) Put(r Record) error {
	if m.records == nil {
		m.records = make(map[string]Record)
	}
	r.Tags = slices.Clone(r.Tags) // r 本身是拷贝，改它不影响调用方
	m.records[r.ID] = r
	return nil
}

// Len 实现 Counter。这个我替你写了，它不是今天的考点 —— 但注意 len(nil map) 是 0，
// 所以零值 MemStore 上调它也安全。
func (m *MemStore) Len() int {
	return len(m.records)
}

// ValidationError 说明一条记录哪个字段不合法。
//
// 用指针接收者实现 Error —— D4 §3 讲过：**指针给你身份语义，值给你相等语义**，
// 错误类型几乎总是要身份语义。
type ValidationError struct {
	// Field 是出问题的字段名。
	Field string
	// Reason 是人话描述的原因。
	Reason string
}

// Error 实现 error。
//
// TODO(D5 主练习)：实现我。
//
// 格式固定为：`字段 ID 不合法：不能为空`
func (e *ValidationError) Error() string {
	return fmt.Sprintf("字段 %s 不合法：%s", e.Field, e.Reason)
}

// Validate 检查 r 是否合法，合法时返回 nil。
//
// TODO(D5 主练习)：实现我。
//
// 规则（按顺序检查，返回第一个错）：
//   - ID 为空          → &ValidationError{Field: "ID", Reason: "不能为空"}
//   - Name 为空        → &ValidationError{Field: "Name", Reason: "不能为空"}
//
// ⚠️ 注意它的返回值类型是 *ValidationError，【不是】error。
// D5 §4 说过这是埋雷的写法，但真实的库里确实有这么写的（调用方想直接读 Field
// 而不用 errors.As）。你必须会安全地处理它 —— 见 Save。
func Validate(r Record) *ValidationError {
	if r.ID == "" {
		return &ValidationError{Field: "ID", Reason: "不能为空"}
	}
	if r.Name == "" {
		return &ValidationError{Field: "Name", Reason: "不能为空"}
	}
	return nil
}

// Save 校验 r，通过后写入 p。
//
// TODO(D5 主练习)：实现我。
//
// ⚠️⚠️ 今天的题眼在这里。下面这个写法是【错的】：
//
//	func Save(p Putter, r Record) error {
//	    return Validate(r)          // ← 记录合法时，返回的 error 也不是 nil！
//	}
//
// 因为 Validate 返回的 nil 是 *ValidationError 类型的 nil，装进 error 接口后
// 变成 (*ValidationError, nil)，而接口只有两格都空才 == nil（D5 §3、§4）。
// 测试里 TestSaveReturnsTrueNil 专门抓这个。
func Save(p Putter, r Record) error {
	vlderr := Validate(r)
	if vlderr != nil {
		return vlderr
	}
	return p.Put(r)
}

// Count 返回 g 里的记录条数；g 不支持计数时返回 -1。
//
// TODO(D5 主练习)：实现我。
//
// 用类型断言探测可选接口 Counter，comma-ok 形式（D5 §5）。
func Count(g Getter) int {
	if counter, ok := g.(Counter); ok {
		return counter.Len()
	}
	return -1
}

// CopyAll 把 ids 对应的记录从 src 拷贝到 dst，返回成功拷贝的条数。
//
// TODO(D5 主练习)：实现我。
//
// 行为要求：
//   - src 里不存在的 id（errors.Is(err, ErrNotFound)）**跳过**，不算失败，继续下一个
//   - src 的其他错误、dst 的任何错误 → **立即返回**，并且用 %w 包装原错误，
//     消息里带上是哪个 id 出的问题
//   - 出错时 copied 返回【出错前已经成功的条数】，不要返回 0
//
// 注意签名：两个参数都是接口，函数体里没有任何一处提到 MemStore ——
// 这就是依赖倒置。测试里我用假实现（fakeStore）注入故障，正因为这里依赖的是接口。
func CopyAll(dst Putter, src Getter, ids []string) (int, error) {
	copied := 0
	for _, id := range ids {
		r, err := src.Get(id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return copied, fmt.Errorf("获取记录 %q: %w", id, err)
		}

		if err := dst.Put(r); err != nil {
			return copied, fmt.Errorf("拷贝记录 %q: %w", id, err)
		}
		copied++
	}
	return copied, nil
}

// 让编译器帮忙确认设计约束：*MemStore 必须同时是 Store 和 Counter。
//
// 这种 `var _ Interface = (*Type)(nil)` 的写法在标准库里到处都是，零运行时开销。
// 注意是 *MemStore 不是 MemStore —— 方法都是指针接收者（D4 §3 方法集）。
var (
	_ Store   = (*MemStore)(nil)
	_ Counter = (*MemStore)(nil)
)
