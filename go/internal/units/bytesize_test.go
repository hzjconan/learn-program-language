package units

import "testing"

// TestUnitConstants 钉住单位常量的数值。
//
// 这个测试的用处在小题 A-1：你把 const 块改写成 iota 版本后，
// 它保证你没算错任何一档。有了它，重构才敢下手 ——
// 这也是「先有测试再重构」在最小尺度上的演示。
func TestUnitConstants(t *testing.T) {
	tests := []struct {
		name string
		got  ByteSize
		want ByteSize
	}{
		{name: "B", got: B, want: 1},
		{name: "KB", got: KB, want: 1 << 10},
		{name: "MB", got: MB, want: 1 << 20},
		{name: "GB", got: GB, want: 1 << 30},
		{name: "TB", got: TB, want: 1 << 40},
		{name: "PB", got: PB, want: 1 << 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}

	// 每一档都应该恰好是上一档的 1024 倍。
	units := []ByteSize{B, KB, MB, GB, TB, PB}
	for i := 1; i < len(units); i++ {
		if units[i] != units[i-1]*1024 {
			t.Errorf("units[%d] = %d, want %d", i, units[i], units[i-1]*1024)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		in   ByteSize
		want string
	}{
		{name: "零", in: 0, want: "0 B"},
		{name: "小于 1KB", in: 512, want: "512 B"},
		{name: "1KB 前一字节", in: 1023, want: "1023 B"},
		{name: "恰好 1KB", in: KB, want: "1.0 KB"},
		{name: "1.5KB", in: KB + KB/2, want: "1.5 KB"},
		{name: "恰好 1MB", in: MB, want: "1.0 MB"},
		{name: "2.5GB", in: 2*GB + GB/2, want: "2.5 GB"},
		{name: "恰好 1TB", in: TB, want: "1.0 TB"},
		{name: "恰好 1PB", in: PB, want: "1.0 PB"},
		{name: "超过 PB 仍用 PB", in: 2048 * TB, want: "2.0 PB"},

		// 负数分支
		{name: "负数小值", in: -512, want: "-512 B"},
		{name: "负数 1.5KB", in: -(KB + KB/2), want: "-1.5 KB"},

		// 边界：1MB 差一字节。
		// 期望值看着别扭（"1024.0 KB" 而不是 "1.0 MB"），但这是规则
		// 「选用不超过该数值的最大单位」+「保留一位小数」直接推出来的结果，
		// 四舍五入发生在选好单位之后。我故意把它钉死在这里 ——
		// 现实中的格式化 bug 十有八九就藏在这种地方，你得先意识到它存在。
		{name: "1MB 差一字节", in: MB - 1, want: "1024.0 KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatBytes(tt.in); got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
