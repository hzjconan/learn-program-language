package slicex

import (
	"slices"
	"testing"
)

// checkIndependent 是本包测试的核心工具：验证 got 和 src 真的没有共享底层数组。
//
// 光看「返回值对不对」抓不到别名 bug —— 结果往往是对的，被毁掉的是【入参】。
// 所以这里做三重检查，一层比一层隐蔽：
//
//	① 调用之后入参有没有被就地改写      → 抓 s[:0] 和 append(s[:i], ...) 这类写法
//	② 往结果里 append 之后入参变没变     → 抓「结果是新的，但 cap 伸进了入参的数组」
//	③ 改结果的元素之后入参变没变         → 抓结果直接是入参子切片的情况
func checkIndependent[T comparable](t *testing.T, src, orig, got []T) {
	t.Helper()

	if !slices.Equal(src, orig) {
		t.Errorf("① 入参被就地改写了\n  src  = %v\n  want = %v\n"+
			"  （是不是用了 s[:0] 复用，或 append(s[:i], s[i+1:]...)？）", src, orig)
		return
	}

	var zero T
	got = append(got, zero) //nolint:gocritic // 故意触发一次写入，检测底层数组是否重叠
	if !slices.Equal(src, orig) {
		t.Errorf("② 往结果 append 之后入参变了\n  src  = %v\n  want = %v\n"+
			"  （结果的 cap 伸进了入参的底层数组）", src, orig)
		return
	}

	if len(got) > 1 {
		got[0] = zero
		if !slices.Equal(src, orig) {
			t.Errorf("③ 修改结果元素之后入参变了\n  src  = %v\n  want = %v\n"+
				"  （结果就是入参的一个子切片）", src, orig)
		}
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		keep func(int) bool
		want []int
	}{
		{name: "留偶数", in: []int{1, 2, 3, 4, 5, 6}, keep: func(v int) bool { return v%2 == 0 }, want: []int{2, 4, 6}},
		{name: "全留", in: []int{1, 2, 3}, keep: func(int) bool { return true }, want: []int{1, 2, 3}},
		{name: "全不留", in: []int{1, 2, 3}, keep: func(int) bool { return false }, want: []int{}},
		{name: "空切片", in: []int{}, keep: func(int) bool { return true }, want: []int{}},
		{name: "nil 切片", in: nil, keep: func(int) bool { return true }, want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := slices.Clone(tt.in)
			got := Filter(tt.in, tt.keep)

			if !slices.Equal(got, tt.want) {
				t.Errorf("Filter(%v) = %v, want %v", tt.in, got, tt.want)
			}
			// 契约里写了「返回非 nil 的空切片」。
			// nil 和空切片对 len/range/append 完全等价，但 encoding/json
			// 会把 nil 编成 null、把空切片编成 []，对 API 使用方是两回事。
			if got == nil {
				t.Error("返回了 nil，契约要求返回非 nil 的空切片")
			}
			checkIndependent(t, tt.in, orig, got)
		})
	}
}

func TestFilterString(t *testing.T) {
	src := []string{"go", "", "rust", "", "zig"}
	orig := slices.Clone(src)

	got := Filter(src, func(s string) bool { return s != "" })

	want := []string{"go", "rust", "zig"}
	if !slices.Equal(got, want) {
		t.Errorf("Filter = %v, want %v", got, want)
	}
	checkIndependent(t, src, orig, got)
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		i    int
		want []int
	}{
		{name: "删中间", in: []int{1, 2, 3, 4, 5}, i: 2, want: []int{1, 2, 4, 5}},
		{name: "删头", in: []int{1, 2, 3}, i: 0, want: []int{2, 3}},
		{name: "删尾", in: []int{1, 2, 3}, i: 2, want: []int{1, 2}},
		{name: "只有一个元素", in: []int{7}, i: 0, want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := slices.Clone(tt.in)
			got := Delete(tt.in, tt.i)

			if !slices.Equal(got, tt.want) {
				t.Errorf("Delete(%v, %d) = %v, want %v", tt.in, tt.i, got, tt.want)
			}
			if got == nil {
				t.Error("返回了 nil，契约要求返回非 nil 的空切片")
			}
			checkIndependent(t, tt.in, orig, got)
		})
	}
}

func TestDeleteOutOfRange(t *testing.T) {
	for _, i := range []int{-1, 3, 100} {
		t.Run("", func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Delete(s, %d) 没有 panic，契约要求越界时 panic", i)
				}
			}()
			_ = Delete([]int{1, 2, 3}, i)
		})
	}
}

func TestDedup(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want []int
	}{
		{name: "保留首次出现的顺序", in: []int{3, 1, 3, 2, 1}, want: []int{3, 1, 2}},
		{name: "无重复", in: []int{1, 2, 3}, want: []int{1, 2, 3}},
		{name: "全重复", in: []int{5, 5, 5}, want: []int{5}},
		{name: "空切片", in: []int{}, want: []int{}},
		{name: "nil 切片", in: nil, want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := slices.Clone(tt.in)
			got := Dedup(tt.in)

			if !slices.Equal(got, tt.want) {
				t.Errorf("Dedup(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if got == nil {
				t.Error("返回了 nil，契约要求返回非 nil 的空切片")
			}
			checkIndependent(t, tt.in, orig, got)
		})
	}
}

// TestNoSharedBackingArray 是一个更狠的整体检查：
// 构造一个 cap 远大于 len 的入参（这是别名 bug 最容易发作的形状），
// 然后往结果里疯狂 append，入参必须纹丝不动。
func TestNoSharedBackingArray(t *testing.T) {
	src := make([]int, 5, 100) // ⚠️ cap 100，富余 95 个位置
	for i := range src {
		src[i] = i
	}
	orig := slices.Clone(src)

	funcs := map[string]func() []int{
		"Filter": func() []int { return Filter(src, func(v int) bool { return v%2 == 0 }) },
		"Delete": func() []int { return Delete(src, 2) },
		"Dedup":  func() []int { return Dedup(src) },
	}

	for name, fn := range funcs {
		t.Run(name, func(t *testing.T) {
			got := fn()
			for i := range 50 {
				got = append(got, i*1000)
			}
			if !slices.Equal(src, orig) {
				t.Errorf("往 %s 的结果里 append 50 次之后，入参被踩烂了\n  src  = %v\n  want = %v",
					name, src, orig)
			}
		})
	}
}
