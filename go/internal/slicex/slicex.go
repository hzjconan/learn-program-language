// Package slicex 提供「不会偷偷改坏入参」的切片工具。
//
// 本包所有函数遵守同一条契约：
//
//	不修改入参，且返回的切片不与入参共享底层数组。
//
// 这条契约听起来理所当然，但 Go 里默认恰恰相反 —— 切片天生共享底层数组，
// 网上流传的「高效」写法（s[:0] 复用、append(s[:i], s[i+1:]...)）
// 全都会就地改写调用方的数据。今天就是要写出遵守契约的版本。
package slicex

import "fmt"

// Filter 返回 s 中所有满足 keep 的元素组成的新切片，保持原有顺序。
//
// s 为 nil 或空时返回一个非 nil 的空切片。
//
// 【硬要求】不得修改 s，返回值不得与 s 共享底层数组。
//
// TODO(D3 主练习)：实现我。
//
// 反面教材（测试会抓住它）：
//
//	out := s[:0]                      // ← 复用入参的底层数组
//	for _, v := range s { ... }        //   过滤结果看着对，但 s 已经被就地改写
//
// 提示：老老实实 make 一个新切片就行。想省一次分配可以先算出结果长度，
// 但别为了省分配去动入参的内存 —— 那是调用方的地盘。
func Filter[T any](s []T, keep func(T) bool) []T {
	out := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// Delete 返回删除下标 i 处元素后的新切片。
//
// i 越界时 panic（和内置的下标越界行为一致，这属于调用方的 bug）。
//
// 【硬要求】不得修改 s，返回值不得与 s 共享底层数组。
//
// TODO(D3 主练习)：实现我。
//
// 反面教材：
//
//	return append(s[:i], s[i+1:]...)   // ← 元素就地前移，s 被改写
//
// 原因：s[:i] 省略了第三个索引，cap 保持原样，所以它有权限写到 i 之后的位置，
// append 发现容量够就【不扩容】，直接在原数组上挪元素。返回值 len 少 1，
// 正好把残留的尾巴挡在视野外 —— 测返回值永远测不出来。
//
// 标准库的 slices.Delete 也是就地修改（1.22 起会把尾巴清零防内存泄漏，
// 但「改坏入参」这条没变）。用之前先想清楚：你是想改，还是想要个新的。
// 本函数要的是后者。
func Delete[T any](s []T, i int) []T {
	if i < 0 || i >= len(s) {
		panic(fmt.Sprintf("slicex.Delete: index %d out of range [0, %d)", i, len(s)))
	}
	out := make([]T, 0, len(s)-1)
	out = append(out, s[:i]...)
	out = append(out, s[i+1:]...)
	return out
}

// Dedup 返回 s 去重后的新切片，保留每个值【首次出现】的位置顺序。
//
// 【硬要求】不得修改 s，返回值不得与 s 共享底层数组。
//
// TODO(D3 主练习)：实现我。
//
// 提示：用 map 做「见过没有」的集合。Go 没有 set 类型，
// 惯用写法是 map[T]struct{} —— 空结构体不占内存，比 map[T]bool 更表意：
//
//	seen := make(map[T]struct{}, len(s))
//	if _, ok := seen[v]; ok { continue }
//	seen[v] = struct{}{}
//
// 约束是 comparable 而不是 any，因为只有可比较的类型才能当 map 的 key。
func Dedup[T comparable](s []T) []T {
	out := make([]T, 0, len(s))
	seen := make(map[T]struct{}, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
