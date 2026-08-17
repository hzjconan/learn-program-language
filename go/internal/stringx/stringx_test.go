package stringx

import (
	"testing"
	"unicode/utf8"
)

func TestReverseRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "空字符串", in: "", want: ""},
		{name: "单字符", in: "a", want: "a"},
		{name: "纯 ASCII", in: "hello", want: "olleh"},
		{name: "中英混合", in: "Go语言", want: "言语oG"},
		{name: "纯中文", in: "日本語abc", want: "cba語本日"},
		{name: "emoji", in: "a👍b", want: "b👍a"},
		{name: "全是多字节", in: "一二三", want: "三二一"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReverseRunes(tt.in)
			if got != tt.want {
				t.Errorf("ReverseRunes(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// 逐字节反转的实现会产出非法 UTF-8，这条断言专门抓它。
			if !utf8.ValidString(got) {
				t.Errorf("ReverseRunes(%q) 产出了非法 UTF-8: %q（是不是按 byte 反转的？）", tt.in, got)
			}
		})
	}
}

func TestReverseRunesTwiceIsIdentity(t *testing.T) {
	// 性质测试：反转两次必须回到原样。
	// 比逐条断言更能抓住实现里的边界错误。
	for _, s := range []string{"", "a", "hello", "Go语言", "日本語abc", "一二三👍"} {
		if got := ReverseRunes(ReverseRunes(s)); got != s {
			t.Errorf("ReverseRunes(ReverseRunes(%q)) = %q, want %q", s, got, s)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{name: "中英混合截断", in: "Go语言很好", n: 3, want: "Go语"},
		{name: "n 超过长度", in: "hello", n: 10, want: "hello"},
		{name: "n 恰好等于长度", in: "hello", n: 5, want: "hello"},
		{name: "n 为 0", in: "hello", n: 0, want: ""},
		{name: "n 为负数", in: "hello", n: -1, want: ""},
		{name: "空字符串", in: "", n: 5, want: ""},
		{name: "纯中文截一个", in: "中文", n: 1, want: "中"},
		{name: "emoji 不能切碎", in: "👍👍👍", n: 2, want: "👍👍"},
		{name: "ASCII", in: "hello", n: 2, want: "he"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateRunes(tt.in, tt.n)
			if got != tt.want {
				t.Errorf("TruncateRunes(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("TruncateRunes(%q, %d) 产出了非法 UTF-8: %q（是不是用 len(s) 和 s[:n]？）",
					tt.in, tt.n, got)
			}
			if n := utf8.RuneCountInString(got); tt.n > 0 && n > tt.n {
				t.Errorf("TruncateRunes(%q, %d) 结果有 %d 个字符，超了", tt.in, tt.n, n)
			}
		})
	}
}

func TestCountRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "空字符串", in: "", want: 0},
		{name: "纯 ASCII", in: "hello", want: 5},
		{name: "中英混合", in: "Go语言", want: 4}, // 而 len() 是 8
		{name: "纯中文", in: "一二三", want: 3},   // 而 len() 是 9
		{name: "emoji", in: "👍", want: 1},   // 而 len() 是 4
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountRunes(tt.in); got != tt.want {
				t.Errorf("CountRunes(%q) = %d, want %d（len() 是 %d，别用它）",
					tt.in, got, tt.want, len(tt.in))
			}
		})
	}
}
