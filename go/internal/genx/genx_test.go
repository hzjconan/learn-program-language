package genx

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Celsius 用来验证约束里的波浪号：~float64 才能让具名类型也传得进来。
type Celsius float64

func TestMap(t *testing.T) {
	got := Map([]int{1, 2, 3}, strconv.Itoa)
	want := []string{"1", "2", "3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Map() = %#v, want %#v", got, want)
	}

	// 类型可以变：int → string 上面试过了，这里 string → int
	lens := Map([]string{"a", "bb", "ccc"}, func(s string) int { return len(s) })
	if !reflect.DeepEqual(lens, []int{1, 2, 3}) {
		t.Errorf("Map(len) = %#v, want [1 2 3]", lens)
	}

	if got := Map(nil, strconv.Itoa); got != nil {
		t.Errorf("Map(nil) = %#v, want nil", got)
	}
	if got := Map([]int{}, strconv.Itoa); got != nil {
		t.Errorf("Map(空切片) = %#v, want nil", got)
	}
}

func TestFilter(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	got := Filter([]int{1, 2, 3, 4, 5, 6}, isEven)
	if !reflect.DeepEqual(got, []int{2, 4, 6}) {
		t.Errorf("Filter(偶数) = %#v, want [2 4 6]", got)
	}

	if got := Filter([]int{1, 3, 5}, isEven); got != nil {
		t.Errorf("Filter(全不满足) = %#v, want nil", got)
	}
	if got := Filter(nil, isEven); got != nil {
		t.Errorf("Filter(nil) = %#v, want nil", got)
	}
}

func TestIndex(t *testing.T) {
	tests := []struct {
		name string
		s    []string
		v    string
		want int
	}{
		{name: "第一个", s: []string{"a", "b", "c"}, v: "a", want: 0},
		{name: "中间", s: []string{"a", "b", "c"}, v: "b", want: 1},
		{name: "重复时取第一个", s: []string{"a", "b", "a"}, v: "a", want: 0},
		{name: "不存在", s: []string{"a", "b"}, v: "z", want: -1},
		{name: "空切片", s: nil, v: "a", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Index(tt.s, tt.v); got != tt.want {
				t.Errorf("Index(%v, %q) = %d, want %d", tt.s, tt.v, got, tt.want)
			}
		})
	}
}

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	got := Keys(m)
	slices.Sort(got) // map 遍历顺序随机，比较前必须排序

	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("Keys() = %#v, want [a b c]", got)
	}

	if got := Keys(map[string]int{}); got != nil {
		t.Errorf("Keys(空 map) = %#v, want nil", got)
	}
	if got := Keys[int, string](nil); got != nil {
		t.Errorf("Keys(nil map) = %#v, want nil", got)
	}
}

func TestSum(t *testing.T) {
	if got := Sum([]int{1, 2, 3}); got != 6 {
		t.Errorf("Sum([]int) = %d, want 6", got)
	}
	if got := Sum([]float64{1.5, 2.5}); got != 4 {
		t.Errorf("Sum([]float64) = %v, want 4", got)
	}
	if got := Sum([]int{}); got != 0 {
		t.Errorf("Sum(空切片) = %d, want 0", got)
	}

	// 关键：具名类型能传进来，靠的是约束里的 ~float64。
	// 把 Number 里的 ~ 去掉，这一行就编译不过了 —— 值得动手试一次。
	if got := Sum([]Celsius{20, 3}); got != 23 {
		t.Errorf("Sum([]Celsius) = %v, want 23", got)
	}
}

func TestGroupBy(t *testing.T) {
	words := []string{"go", "rust", "c", "java", "js", "zig"}
	got := GroupBy(words, func(s string) int { return len(s) })

	want := map[int][]string{
		1: {"c"},
		2: {"go", "js"},
		3: {"zig"},
		4: {"rust", "java"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GroupBy(按长度) = %#v\nwant %#v\n（每组内部要保持原始顺序）", got, want)
	}

	// 分组键也可以是别的类型
	byFirst := GroupBy(words, func(s string) string { return strings.ToUpper(s[:1]) })
	if len(byFirst["J"]) != 2 {
		t.Errorf("byFirst[\"J\"] = %v, want 2 个元素", byFirst["J"])
	}

	if got := GroupBy(nil, func(s string) int { return len(s) }); got != nil {
		t.Errorf("GroupBy(nil) = %#v, want nil", got)
	}
}

// TestSetZeroValueUsable 验证泛型类型的零值同样应该可用（D4 §1）。
func TestSetZeroValueUsable(t *testing.T) {
	var s Set[string] // 没有 NewSet()

	if s.Len() != 0 {
		t.Errorf("零值 Set.Len() = %d, want 0", s.Len())
	}
	if s.Has("a") {
		t.Error("零值 Set 不该包含任何元素")
	}

	s.Add("a")
	s.Add("b")
	s.Add("a") // 重复加入

	if s.Len() != 2 {
		t.Errorf("Add 两个不同元素 + 一个重复后 Len() = %d, want 2", s.Len())
	}
	if !s.Has("a") || !s.Has("b") {
		t.Error("Has 应该对已加入的元素返回 true")
	}
	if s.Has("z") {
		t.Error("Has 应该对没加入的元素返回 false")
	}
}

func TestSetOtherTypes(t *testing.T) {
	// comparable 约束下，struct 也能当元素 —— 前提是它所有字段都可比较（D4 §1）
	type point struct{ X, Y int }

	var s Set[point]
	s.Add(point{1, 2})
	s.Add(point{1, 2})

	if s.Len() != 1 {
		t.Errorf("相同的 struct 应该被去重：Len() = %d, want 1", s.Len())
	}
	if !s.Has(point{1, 2}) {
		t.Error("Has(point{1,2}) = false, want true")
	}
}
