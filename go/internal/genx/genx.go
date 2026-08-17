// Package genx 是一组泛型工具函数，D5 的副练习。
//
// 重点不是写出来（每个都只有几行），是**体会约束怎么选**：
// 什么时候 any 就够、什么时候必须 comparable、什么时候要自己写类型集。
//
// # 现实提醒
//
// slices.Index、slices.Sort、maps.Keys 标准库已经有了
// （maps.Keys 在 Go 1.23+ 返回的是迭代器 iter.Seq[K]，不是切片）。
// **真实项目里别自己写这些。** 这里手写一遍纯粹是为了理解约束。
//
// 而 Map / Filter 标准库【故意】没有 —— Go 团队认为直接写 for 循环更清楚。
// 这也是一种态度：泛型是工具，不是越多越好。
package genx

// Number 是一个类型集约束。
//
// 波浪号 ~int 表示「底层类型是 int 的所有类型」，这样 D1 里
// `type Celsius float64` 这种具名类型也能传进来。**几乎总是要写 ~。**
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~float32 | ~float64
}

// Map 对 s 的每个元素应用 f，返回等长的新切片。
//
// TODO(D5 副练习)：实现我。
//
// 约定：len(s) == 0 时返回 nil（保持 D3 那条「nil 切片就是好的空切片」）。
//
// 想想为什么约束是 any 而不是 comparable：函数体里从没比较过元素。
// **约束要选够用的最弱的那个** —— 选强了，能传进来的类型就变少了。
func Map[T, U any](s []T, f func(T) U) []U {
	if len(s) == 0 {
		return nil
	}
	mapped := make([]U, len(s))
	for i, v := range s {
		mapped[i] = f(v)
	}
	return mapped
}

// Filter 返回 s 中满足 keep 的元素组成的新切片。
//
// TODO(D5 副练习)：实现我。
//
// 约定：len(s) == 0 或没有元素满足条件时返回 nil。
func Filter[T any](s []T, keep func(T) bool) []T {
	filtered := make([]T, 0, len(s))
	for _, v := range s {
		if keep(v) {
			filtered = append(filtered, v)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// Index 返回 v 在 s 中第一次出现的下标；不存在时返回 -1。
//
// TODO(D5 副练习)：实现我。
//
// 这里约束必须是 comparable —— 因为函数体里要用 ==。
// 试试改成 any，看编译器怎么骂你。
func Index[T comparable](s []T, v T) int {
	for i, t := range s {
		if t == v {
			return i
		}
	}
	return -1
}

// Keys 返回 m 的所有键。顺序【不确定】（map 遍历顺序是随机的，D3）。
//
// TODO(D5 副练习)：实现我。
//
// 约定：len(m) == 0 时返回 nil。
//
// 注意 K 必须是 comparable —— 但这不是我们要求的，是 map 的键本来就必须可比较，
// 编译器会强制你写。V 则完全用不到，所以是 any。
func Keys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// Sum 返回 s 中所有元素之和。
//
// TODO(D5 副练习)：实现我。
//
// 提示：`var total T` 是泛型里拿零值的标准写法 —— 你不知道 T 是什么，
// 没法写 0 这样的字面量（其实这里能写，但换成别的类型就不行了，养成习惯）。
func Sum[T Number](s []T) T {
	var total T
	for _, v := range s {
		total += v
	}
	return total
}

// GroupBy 按 key(v) 的结果把 s 分组。
//
// TODO(D5 副练习)：实现我。
//
// 约定：len(s) == 0 时返回 nil。每组内部保持 s 里的原始顺序。
//
// 这个函数用到了两个类型参数，且约束不同：T 只是被搬运（any），
// K 要当 map 的键（comparable）。
func GroupBy[T any, K comparable](s []T, key func(T) K) map[K][]T {
	if len(s) == 0 {
		return nil
	}
	grouped := make(map[K][]T)
	for _, v := range s {
		k := key(v)
		grouped[k] = append(grouped[k], v)
	}
	return grouped
}

// Set 是基于 map 的集合，零值可用（D4 §1）。
//
// 值类型用 struct{} 而不是 bool：零尺寸，且表意更准确 —— 我们只关心「在不在」，
// 不关心对应什么值。详见 QA #11。
//
// # 并发安全性：不安全
//
// Add 与任何方法并发调用，都必须由调用方自己加锁。**并发只读（只调 Has / Len）
// 是安全的** —— Go 保证 map 在没有写入者时可以被并发读。
//
// 这是 Go 的默认约定，不是缺陷：bytes.Buffer、strings.Builder、map 本身都是如此，
// 由调用方决定要不要付同步的代价。需要并发安全时另开一个类型（内嵌 sync.RWMutex），
// 别把锁塞进这里让所有单线程用户买单。
type Set[T comparable] struct {
	m map[T]struct{}
}

// Add 把 v 加入集合，重复加入无副作用。
//
// TODO(D5 副练习)：实现我。
//
// 要点：惰性初始化 s.m，这样 `var s Set[string]` 直接就能用。
func (s *Set[T]) Add(v T) {
	if s.m == nil {
		s.m = make(map[T]struct{})
	}
	s.m[v] = struct{}{}
}

// Has 报告 v 是否在集合里。
//
// TODO(D5 副练习)：实现我。
func (s *Set[T]) Has(v T) bool {
	_, ok := s.m[v]
	return ok
}

// Len 返回集合大小。这个我替你写了 —— len(nil map) 是 0，所以零值上调也安全。
func (s *Set[T]) Len() int {
	return len(s.m)
}
