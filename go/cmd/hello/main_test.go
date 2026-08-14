package main

import (
	"slices"
	"testing"
)

// TestGreet 是表驱动测试（table-driven test）的最小样板。
// 这是 Go 社区绝对主流的测试写法，本周你会反复写它 —— 现在先看清结构。
func TestGreet(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "普通名字", in: "Go", want: "Hello, Go!"},
		{name: "中文", in: "世界", want: "Hello, 世界!"},
		{name: "空字符串", in: "", want: "Hello, !"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := greet(tt.in)
			if got != tt.want {
				// Go 没有断言库，标准做法就是 if + t.Errorf，
				// 且惯例是 "got X, want Y" 这个措辞顺序。
				t.Errorf("greet(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestToolchainFeatures 验证新版工具链特性可用。
// 这个测试在 Go 1.17 上连编译都过不去 —— 它就是我们升级环境的理由。
func TestToolchainFeatures(t *testing.T) {
	// 1.21: min / max 成为内置函数
	if got := max(3, 7, 5); got != 7 {
		t.Errorf("max = %d, want 7", got)
	}

	// 1.21: slices 标准库包（泛型实现）
	s := []int{3, 1, 2}
	slices.Sort(s)
	if !slices.Equal(s, []int{1, 2, 3}) {
		t.Errorf("sorted = %v, want [1 2 3]", s)
	}

	// 1.22: range over int + 循环变量每轮独立
	// 在 1.21 及更早版本上，下面三个闭包会全部返回 3 —— 这是 Go 史上最著名的坑。
	// 现在每轮迭代的 i 是独立变量，闭包捕获到的是各自那一份。
	var fns []func() int
	for i := range 3 {
		fns = append(fns, func() int { return i })
	}
	var got []int
	for _, fn := range fns {
		got = append(got, fn())
	}
	if !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("闭包捕获结果 = %v, want [0 1 2]（若为 [3 3 3] 说明 Go 版本 < 1.22）", got)
	}
}
